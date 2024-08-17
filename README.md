### Sincronizador/Monitorador

#### Este WebSocket em Go foi desenvolvido para resolver diversos problemas, como quedas de VMs que suportam bancos de dados, evitando a perda de informações importantes.

#### Basicamente, ele monitora constantemente dois bancos de dados simultaneamente. Além disso, ele recebe as informações que seriam enviadas diretamente para um único banco de dados.

#### Com este WebSocket, é possível configurar um segundo banco de dados como um backup em casos de falhas no banco de dados primário. O WebSocket recebe as informações e as duplica, enviando uma cópia para o banco de dados primário e outra para o banco de dados secundário. Dessa forma, quando o banco de dados primário falha, as informações continuam sendo validadas e enviadas para o banco de dados secundário. Todas as informações enviadas para o banco de dados secundário são armazenadas em um buffer. Assim, quando o banco de dados primário retorna, essas informações são automaticamente enviadas para ele.

#### Inicialmente, foi criada uma página para validar a disponibilidade dos bancos de dados. No entanto, o monitoramento dos logs é realizado via prompt de comando.
