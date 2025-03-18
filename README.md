To pause one server use the following command:
    pkill -STOP -P <process_id>

To resume one server use the following command:
    pkill -CONT -P <process_id>

To kill a server use:
    pkill -P <process_id>

To run the cluster just use the following command in raft folder:
    go run main.go