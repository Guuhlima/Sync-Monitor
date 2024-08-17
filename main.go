package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
)

var (
	primaryDB     *sql.DB
	secondaryDB   *sql.DB
	bufferChannel = make(chan map[string]interface{}, 100000)
	upgrader      = websocket.Upgrader{}
	primaryDown   = false
	secondaryDown = false
	bufferMutex   sync.Mutex
)

func main() {
	var err error
	primaryDB, err = sql.Open("mysql", "root:suaSenha@tcp(Seuipbancoprimario)/databasePrimaria")
	if err != nil {
		log.Fatalf("Erro ao abrir a conexão com o banco primário: %v", err)
	}
	defer primaryDB.Close()

	secondaryDB, err = sql.Open("mysql", "root:suaSenha@tcp(Seuipbancosecundario)/databaseSecundaria")
	if err != nil {
		log.Fatalf("Erro ao abrir a conexão com o banco secundário: %v", err)
	}
	defer secondaryDB.Close()

	http.HandleFunc("/ws", handleConnection)
	go monitorDatabases()
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Erro ao estabelecer conexão WebSocket:", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Println("Conexão fechada normalmente")
			} else {
				log.Println("Erro ao ler mensagem:", err)
			}
			break
		}

		log.Printf("Mensagem recebida: %s\n", msg)

		var data map[string]interface{}
		if err := json.Unmarshal(msg, &data); err != nil {
			log.Println("Erro ao deserializar mensagem:", err)
			continue
		}

		matricula, _ := data["matricula"].(string) // Supondo que 'matricula' está presente na mensagem
		acao, _ := data["acao"].(string)           // Supondo que 'acao' está presente na mensagem
		hostname, _ := data["hostname"].(string)   // Supondo que 'hostname' está presente na mensagem
		ip, _ := data["ip"].(string)               // Supondo que 'ip' está presente na mensagem

		horaSaida, ok1 := data["hora_saida"].(string)
		horaRetorno, ok2 := data["hora_retorno"].(string)
		tipoDePausa, ok3 := data["tipo_de_pausa"].(float64) // JSON decodifica números como float64

		if !ok1 || !ok2 || !ok3 {
			log.Println("Campos 'hora_saida', 'hora_retorno' ou 'tipo_de_pausa' não encontrados ou formatos incorretos")
			continue
		}

		// Verifica se os dados podem ser inseridos diretamente
		entradaSaidaErr := insertEntradaSaida(primaryDB, horaSaida)
		pausaLogErr := insertPausaLog(primaryDB, horaSaida, horaRetorno, int(tipoDePausa))

		entradaSaidaErrSec := insertEntradaSaida(secondaryDB, horaSaida)
		pausaLogErrSec := insertPausaLog(secondaryDB, horaSaida, horaRetorno, int(tipoDePausa))

		if entradaSaidaErr != nil || pausaLogErr != nil || entradaSaidaErrSec != nil || pausaLogErrSec != nil {
			bufferChannel <- data
			if entradaSaidaErr != nil || pausaLogErr != nil {
				log.Println("Erro ao inserir dados nas tabelas em ambos os bancos, armazenando no buffer:", entradaSaidaErr, pausaLogErr)
			}
			if entradaSaidaErrSec != nil || pausaLogErrSec != nil {
				log.Println("Erro ao inserir dados nas tabelas no banco secundário, armazenando no buffer:", entradaSaidaErrSec, pausaLogErrSec)
			}
		}

		// Inserir log
		logErr := insertLog(primaryDB, matricula, acao, hostname, ip)
		if logErr != nil {
			log.Println("Erro ao inserir log no banco primário:", logErr)
		}

		logErrSec := insertLog(secondaryDB, matricula, acao, hostname, ip)
		if logErrSec != nil {
			log.Println("Erro ao inserir log no banco secundário:", logErrSec)
		}
	}
}
func insertEntradaSaida(db *sql.DB, dataField string) error {
	_, err := db.Exec("INSERT INTO entrada_saida (data) VALUES (?)", dataField)
	return err
}

func insertPausaLog(db *sql.DB, horaSaida, horaRetorno string, tipoDePausa int) error {
	_, err := db.Exec("INSERT INTO pausa_log (hora_saida, hora_retorno, tipo_de_pausa) VALUES (?, ?, ?)", horaSaida, horaRetorno, tipoDePausa)
	return err
}

func insertLog(db *sql.DB, matricula, acao, hostname, ip string) error {
	_, err := db.Exec("INSERT INTO logs (matricula, acao, hostname, ip) VALUES (?, ?, ?, ?)", matricula, acao, hostname, ip)
	return err
}

func monitorDatabases() {
	for {
		time.Sleep(10 * time.Second)
		if err := checkDatabase(primaryDB); err != nil {
			if !primaryDown {
				log.Println("Banco primário está fora de operação. Dados serão armazenados em buffer.")
				primaryDown = true
			}
		} else {
			if primaryDown {
				log.Println("Banco primário voltou online. Sincronizando buffer.")
				syncBuffer(primaryDB)
				primaryDown = false
			}
		}

		if err := checkDatabase(secondaryDB); err != nil {
			if !secondaryDown {
				log.Println("Banco secundário está fora de operação. Dados serão armazenados em buffer.")
				secondaryDown = true
			}
		} else {
			if secondaryDown {
				log.Println("Banco secundário voltou online. Sincronizando buffer.")
				syncBuffer(secondaryDB)
				secondaryDown = false
			}
		}
	}
}

func checkDatabase(db *sql.DB) error {
	return db.Ping()
}

func syncBuffer(db *sql.DB) {
	bufferMutex.Lock()
	defer bufferMutex.Unlock()

	for {
		select {
		case data := <-bufferChannel:
			matricula, _ := data["matricula"].(string) // Supondo que 'matricula' está presente no buffer
			acao, _ := data["acao"].(string)           // Supondo que 'acao' está presente no buffer
			hostname, _ := data["hostname"].(string)   // Supondo que 'hostname' está presente no buffer
			ip, _ := data["ip"].(string)               // Supondo que 'ip' está presente no buffer

			horaSaida, ok1 := data["hora_saida"].(string)
			horaRetorno, ok2 := data["hora_retorno"].(string)
			tipoDePausa, ok3 := data["tipo_de_pausa"].(float64) // JSON decodifica números como float64

			if !ok1 || !ok2 || !ok3 {
				log.Println("Campos 'hora_saida', 'hora_retorno' ou 'tipo_de_pausa' não encontrados ou formatos incorretos no buffer")
				continue
			}

			// Inserir na tabela 'entrada_saida'
			err := insertEntradaSaida(db, horaSaida)
			if err != nil {
				log.Println("Erro ao sincronizar buffer com a tabela 'entrada_saida':", err)
			} else {
				log.Println("Dados de 'entrada_saida' do buffer sincronizados com o banco.")
			}

			// Inserir na tabela 'pausa_log'
			err = insertPausaLog(db, horaSaida, horaRetorno, int(tipoDePausa))
			if err != nil {
				log.Println("Erro ao sincronizar buffer com a tabela 'pausa_log':", err)
			} else {
				log.Println("Dados de 'pausa_log' do buffer sincronizados com o banco.")
			}

			// Inserir na tabela 'logs'
			err = insertLog(db, matricula, acao, hostname, ip)
			if err != nil {
				log.Println("Erro ao sincronizar buffer com a tabela 'logs':", err)
			} else {
				log.Println("Dados de 'logs' do buffer sincronizados com o banco.")
			}

		default:
			return
		}
	}
}
