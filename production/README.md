# Production setup
Firs pick how many servers you want to have in your cluster. Choose an odd number. Then create the choosen number ob servers/VMs. On every VM clone this repository.

We are going to demonstrate the setup for three servers. On each server create configuration.json file inside production folder. Each file should contain the following content: 
```
{
  "numberOfServers": 3,
  "serverIndex": 0,
  "serverAddresses": ["192.168.2.190:5000", "192.168.2.191:5000", "192.168.2.192:5000"]
}
```
The numberOfServers specifies the number of servers in the cluster. serverIndex specifies which server address in serverAddresses represents current server. serverAddresses is an array that contains all server addresses in the cluster, default port of the program is 5000. After you create prodConfiguration.json on all servers, you can run the following command inside production folder on each server:
```
go run main.go
```
Servers will then start communicating to each other and they will elect a leader. After successful election outputX.txt file will contain the address of the leader to which you can send data to be replicated.

For easier data replication to the cluster test.go file is included. You need to correct the server addres on line 25. Inside of it you will find numOfGoRutines variable, which specifies how many rutines should simultaneously send the requests to the leader. numOfClientRequests specifies how many request each rutine should send to the leader.