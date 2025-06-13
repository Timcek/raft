# Stopping and restarting processes
To pause one server use the following command:
```
pkill -STOP -P <process_id>
```

To resume one server use the following command:
```
pkill -CONT -P <process_id>
```

To kill a server use:
```
pkill -P <process_id>
```

To run the cluster just use the following command in raft folder:
```
go run main.go
```

To watch a log file use the following command:
```
watch -n 1 -d tail output127.0.0.1\:<server_port>.txt
```

To specify custom configuration file to program use the following command:
```
go run main.go <configuration_file>
```

# Visualization setup

Checkout the visualization branch.

Then move to viz folder and execute the following command. This clones the submodules necessary for running the visualization:
```
git submodule update --init --recursive
```

I recommend you to run the program using docker containers (The program uses OS processes for creating server nodes and 
web sockets for communicating with frontend. This can cause some problems when killing the process, that is why it is 
better to use containers since the program is isolated from the host machine.). Docker compose file is located at the root 
of this repository. All you need to do is run the following commands in the root directory of this project:
```
docker compose build
docker compose up
```

All that is left to do is to open index.html (that is inside viz folder). This opens up the Raft visualization. By 
pressing the button "Začni simulacijo" the visualization will begin with the default parameters (when running the 
container for the first time it can take some time for the visualization to begin. Just wait for the visualization to 
start working. This happens because for some reason Docker takes some time to create new processes inside the container).

If you want to create a custom visualization configuration you need to select a JSON file inside "Izberi datoteko" field and 
press "Začni simulacijo". Below is an example of a JSON file for 5 servers configuration. Every configuration must satisfy
the next conditions:
- At the top of the file there must be a properties for number of servers and the next term. The nextTerm value should represent
the upcoming term number. For example, if the highest term in serverLogs is 5, then nextTerm should be set to 6 or higher.
- The serverLogs property must be an array of arrays. Each inner array represents the log entries for e server. Each 
log entry should be an object with the following properties:
  - Commited: indicates whether the log entry is commited. 
  - Term: Specifies the term in which the entry was created. 
  - Index: Should start at 1 and reset to 1 every time the term increases.

```
{
  "numberOfServers": 5,
  "nextTerm": 4,
  "serverLogs":
  [
    [
      {
        "commited": true,
        "term": 1,
        "index": 1
      },
      {
        "commited": false,
        "term": 2,
        "index": 1
      }
    ],
    [
      {
        "commited": true,
        "term": 1,
        "index": 1
      },
      {
        "commited": false,
        "term": 2,
        "index": 1
      }
    ],
    [
      {
        "commited": true,
        "term": 1,
        "index": 1
      }
    ],
    [
      {
        "commited": true,
        "term": 1,
        "index": 1
      }
    ],
    [
      {
        "commited": true,
        "term": 1,
        "index": 1
      },
      {
        "commited": false,
        "term": 3,
        "index": 1
      }
    ]
  ]
}
```

## Visualization shorkeys
- Press S to stop the simulation
- Press R to resume the simulation
- Press C to trigger default client request