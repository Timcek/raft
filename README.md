To pause one server use the following command:
    pkill -STOP -P <process_id>

To resume one server use the following command:
    pkill -CONT -P <process_id>

To kill a server use:
    pkill -P <process_id>

To run the cluster just use the following command in raft folder:
    go run main.go

To watch a log file use the following command:
watch -n 1 -d tail output127.0.0.1\:<server_port>.txt

To specify custom configuration file to program use the following command:
go run main.go <configuration_file>