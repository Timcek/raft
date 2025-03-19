package server

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"math/rand"
	"net"
	"os"
	"raft/log"
	sgrpc "raft/server/src"
	"sync"
	"time"
)

const LEADER = 1
const FOLLOWER = 2
const CANDIDATE = 3

// This means the number of milliseconds
const electionTimeoutTime = 16000

type Server struct {
	sgrpc.UnimplementedServerServiceServer
	currentTerm int
	// Tells us if this server already voted in currentTerm
	votedInThisTerm bool
	log             []log.Message
	// LEADER or FOLLOWER or CANDIDATE
	serverState int

	leaderAddress string // TODO to prestavlja address od trenutnega leaderja. Potrebno implementirati, da bo kliente preusmerilo nanj
	// serverAddresses contains addresses of the servers in the cluster without this server
	serverAddresses []string
	// Current server address, used for other servers to set leader to this address, when this server is elected for leader
	serverAddress string

	// Timers used for triggering elections and heartbeat
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer

	// Election parameters
	voteCount  int
	inElection bool

	// Locks needed for concurrent vote request and heartbeat sending
	electionMutex         sync.Mutex
	heartbeatMutex        sync.Mutex
	clientRequestMutex    sync.Mutex
	issueAppendEntryMutex sync.Mutex

	//TODO tole je vse potrebno pri replikaciji sporočil ne pri volitvah.
	//volatile state on every server
	commitIndex int

	//TODO tale lastApplied ni nujno potreben
	lastApplied int
	//volatile state on leader
	nextIndex []int

	// Index 0 is reserved for current server and each of the other indexes is reserved for servers specified in serverAddresses
	matchIndex []int

	// Opened file for writing all the messages on the server. This enables us to easier keep track of what is going on in servers.
	file *os.File
}

// Server constructor

func CreateServer(address string, addresses []string) {
	// Create server with starting values
	server := Server{}
	server.currentTerm = 0
	server.votedInThisTerm = false
	server.serverState = FOLLOWER
	//TODO tole preglej ko boš delal log če je uredu
	server.commitIndex = 0
	server.serverAddress = address
	server.serverAddresses = addresses
	server.createElectionTimer()
	server.matchIndex = make([]int, len(server.serverAddresses)+1)

	file, err := os.Create("output" + address + ".txt")
	defer file.Close()
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	server.file = file

	// Register grpc methods
	grpcServer := grpc.NewServer()
	sgrpc.RegisterServerServiceServer(grpcServer, &server)
	// odpremo vtičnico
	listener, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}
	server.writeToFile("gRPC server listening at " + address + "\n")
	// začnemo s streženjem
	if err := grpcServer.Serve(listener); err != nil {
		panic(err)
	}
}

// Election timer management. When the timer triggers the election process starts.

func (server *Server) createElectionTimer() {
	randomNum := rand.Intn(electionTimeoutTime/2) + electionTimeoutTime/2
	server.electionTimer = time.NewTimer(time.Millisecond * time.Duration(randomNum))
	go func() {
		for true {
			<-server.electionTimer.C
			server.electionTimeout()
			server.resetElectionTimer()
		}
	}()
}

func (server *Server) resetElectionTimer() {
	randomNum := rand.Intn(electionTimeoutTime/2) + electionTimeoutTime/2
	server.electionTimer.Reset(time.Millisecond * time.Duration(randomNum))
}

func (server *Server) stopElectionTimer() {
	if server.electionTimer != nil {
		server.electionTimer.Stop()
	}
}

func (server *Server) electionTimeout() {
	server.becomeCandidate()
	server.beginElection()
}

// Begin election process, send votes, wait for responses and change server state based on vote result.

func (server *Server) beginElection() {
	server.increaseTerm()
	server.inElection = true
	server.resetVoteCountAndVoteForYourself()
	server.issueVoteRequestsToOtherServers()
	server.resetElectionTimer()
	server.writeToFile("Election began " + server.serverAddress + "\n")
}

func (server *Server) increaseTerm() {
	server.currentTerm = server.currentTerm + 1
	server.votedInThisTerm = false
}

func (server *Server) resetVoteCountAndVoteForYourself() {
	// Resets vote count to 0 and votes for himself. The vote count at the end is 1.
	server.voteCount = 1
	server.votedInThisTerm = true
}

func (server *Server) issueVoteRequestsToOtherServers() {
	for _, address := range server.serverAddresses {
		go func() {
			conn, err := grpc.NewClient(address, grpc.WithInsecure())
			if err != nil {
				//TODO tole ni uredu naj nebo panic
				panic(err)
			}
			defer conn.Close()

			grpcClient := sgrpc.NewServerServiceClient(conn)

			contextServer, cancel := context.WithTimeout(context.Background(), time.Millisecond*(electionTimeoutTime/8))
			defer cancel()

			lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
			requestVote := sgrpc.RequestVoteMessage{
				Term:         int64(server.currentTerm),
				LastLogTerm:  lastLogTerm,
				LastLogIndex: lastLogIndex,
			}
			voteResult, err := grpcClient.RequestVote(contextServer, &requestVote)
			if err != nil {
				fmt.Println(err)
			} else {
				server.processVoteResult(voteResult)
			}
		}()
	}
}

func (server *Server) processVoteResult(message *sgrpc.RequestVoteResponse) {
	if !server.inElection {
		return
	}
	server.electionMutex.Lock()
	if message.VoteGranted {
		server.voteCount++
	} else {
		server.receivesDeniedVote(int(message.Term))
	}
	// We always use odd number of servers so if the number of votes is greater than serverAddresses/2 the majority has voted for this server to become a leader.
	if server.voteCount > len(server.serverAddresses)/2 {
		// This server won the elections
		server.becomeLeader()
	}
	server.electionMutex.Unlock()
}

func (server *Server) receivesDeniedVote(term int) {
	// if the term received in a message is higher than the server's current term we have to update it and stop elections.
	if term > server.currentTerm {
		server.changeTerm(term, false)
		server.inElection = false
		server.resetElectionTimer()
	}
}

// Leader heartbeat management

func (server *Server) manageHeartbeat() {
	server.heartbeatTimer = time.NewTimer(time.Millisecond * (electionTimeoutTime / 4))
	go func() {
		for true {
			<-server.heartbeatTimer.C
			server.heartbeatTimeout()
			server.resetHeartbeat()
		}
	}()
}

func (server *Server) stopHeartbeat() {
	if server.heartbeatTimer != nil {
		server.heartbeatTimer.Stop()
	}
}

func (server *Server) resetHeartbeat() {
	// This function is different then in election timeout, because the election timout is first set when the server is
	// started but the heartbeat timeout is first set when the server becomes the leader for the first time
	if server.heartbeatTimer == nil {
		server.manageHeartbeat()
	} else {
		server.heartbeatTimer.Reset(time.Millisecond * (electionTimeoutTime / 4))
	}
}

func (server *Server) heartbeatTimeout() {
	server.writeToFile("Sending heartbeat " + fmt.Sprintf("%v\n", server.log))
	for _, address := range server.serverAddresses {
		go func() {
			conn, err := grpc.NewClient(address, grpc.WithInsecure())
			if err != nil {
				panic(err)
			}
			defer conn.Close()

			grpcClient := sgrpc.NewServerServiceClient(conn)

			contextServer, cancel := context.WithTimeout(context.Background(), time.Millisecond*(electionTimeoutTime/8))
			defer cancel()

			//TODO tale log index potrebno popraviti ko bom delal log replication
			lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
			heartbeatMessage := sgrpc.AppendEntryMessage{
				Term:          int64(server.currentTerm),
				LeaderAddress: server.serverAddress,
				PrevLogIndex:  lastLogIndex,
				PrevLogTerm:   lastLogTerm,
				Entry:         nil,
				//TODO tale commit je potrebno narediti ko se bo dodajalo loge
				LeaderCommit: int64(server.commitIndex),
			}
			heartbeatResponse, err := grpcClient.AppendEntry(contextServer, &heartbeatMessage)
			if err != nil {
				fmt.Println(err)
			} else {
				server.heartbeatMutex.Lock()
				server.processHeartbeatResponse(heartbeatResponse)
				server.heartbeatMutex.Unlock()
			}
		}()
	}
}

func (server *Server) processHeartbeatResponse(response *sgrpc.AppendEntryResponse) {
	if response.Success == false && int(response.Term) > server.currentTerm {
		server.becomeCandidate()
		server.changeTerm(int(response.Term), false)
	}
}

// Change server state

func (server *Server) becomeCandidate() {
	server.inElection = false
	server.serverState = CANDIDATE
	server.stopHeartbeat()
	server.resetElectionTimer()
}

func (server *Server) becomeFollower(leaderAddress string) {
	server.inElection = false
	server.serverState = FOLLOWER
	server.stopHeartbeat()
	server.resetElectionTimer()
	server.leaderAddress = leaderAddress
}

func (server *Server) becomeLeader() {
	server.serverState = LEADER
	server.inElection = false
	server.matchIndex = make([]int, len(server.serverAddresses)+1)
	server.stopElectionTimer()
	server.resetHeartbeat()
}

// Receive and respond to AppendEntry

func (server *Server) AppendEntry(ctx context.Context, in *sgrpc.AppendEntryMessage) (*sgrpc.AppendEntryResponse, error) {
	if int(in.Term) < server.currentTerm {
		return server.receivesHeartbeatOrAppendEntryWithStaleTerm(), nil
	}
	server.replaceServersCurrentTermIfReceivedTermInHeartbeatIsHigher(int(in.Term), in.LeaderAddress)
	// The term received in message is equal or higher than the currentTerm, so the server should become a follower
	server.becomeFollower(in.LeaderAddress)
	if in.Entry == nil {
		server.writeToFile(time.Now().String() + " Received heartbeat, current term: " + fmt.Sprintf("%d", server.currentTerm) + " " + fmt.Sprintf("%v", server.log) + "\n")
		// Heartbeat contains 0 log entries
		return server.receivedValidHeartbeat(), nil
	} else {
		server.writeToFile("Received append entry\n")
		return server.receivedValidAppendEntry(in.Entry), nil
	}
}

func (server *Server) receivesHeartbeatOrAppendEntryWithStaleTerm() *sgrpc.AppendEntryResponse {
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: false,
	}
}

func (server *Server) replaceServersCurrentTermIfReceivedTermInHeartbeatIsHigher(term int, leaderAddress string) {
	if term > server.currentTerm {
		server.changeTerm(term, false)
	}
}

func (server *Server) receivedValidHeartbeat() *sgrpc.AppendEntryResponse {
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: true,
	}
}

func (server *Server) receivedValidAppendEntry(newLogEntry *sgrpc.LogEntry) *sgrpc.AppendEntryResponse {
	server.resetElectionTimer()
	server.log = append(server.log, log.Message{
		Term:  int(newLogEntry.Term),
		Index: int(newLogEntry.Index),
		Msg:   newLogEntry.Message,
	})
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: true,
	}
}

// Receive and respond to vote request

func (server *Server) RequestVote(ctx context.Context, in *sgrpc.RequestVoteMessage) (*sgrpc.RequestVoteResponse, error) {
	server.writeToFile("Received vote request\n")
	server.replaceServersCurrentTermIfReceivedTermInRequestVoteIsHigher(int(in.Term))

	// Retrieve servers last log message (if the log is empty, the term and index should be 0, which they are by default when we create logMessage)
	var lastLog log.Message
	if len(server.log) > 0 {
		lastLog = server.log[len(server.log)-1]
	}

	// Voters term is higher than candidates
	if int(in.Term) < server.currentTerm {
		return &sgrpc.RequestVoteResponse{
			Term:        int64(server.currentTerm),
			VoteGranted: false,
		}, nil
	}

	// Voters log is more up-to-date than the candidates log
	//TODO tole še preglej ko boš delal log replication moraš spreminjati lastLogIndex in lastLogTerm
	if int(in.LastLogTerm) < lastLog.Term || (int(in.LastLogTerm) == lastLog.Term && int(in.LastLogIndex) < lastLog.Index) {
		return &sgrpc.RequestVoteResponse{
			Term:        int64(server.currentTerm),
			VoteGranted: false,
		}, nil
	}

	// Voter already voted in this term
	if server.votedInThisTerm {
		return &sgrpc.RequestVoteResponse{
			Term:        int64(server.currentTerm),
			VoteGranted: false,
		}, nil
	}

	//Voter votes for the candidate
	server.resetElectionTimer()
	server.becomeCandidate()
	server.changeTerm(int(in.Term), true)
	return &sgrpc.RequestVoteResponse{
		Term:        int64(server.currentTerm),
		VoteGranted: true,
	}, nil
}

func (server *Server) replaceServersCurrentTermIfReceivedTermInRequestVoteIsHigher(term int) {
	if term > server.currentTerm {
		server.becomeCandidate()
		server.changeTerm(term, false)
	}
}

// Change term

func (server *Server) changeTerm(term int, votedInThisTerm bool) {
	server.currentTerm = term
	server.votedInThisTerm = votedInThisTerm
}

// Retrieve last log info

func (server *Server) retrieveLastLogIndexAndTerm() (int64, int64) {
	var lastLogTerm int64
	var lastLogIndex int64
	if len(server.log) == 0 {
		// If the servers log is empty we can use 0 for last log term and index, because all servers have these two
		// values set to 0 at the start, and they won't refuse the messages with these values
		lastLogTerm = 0
		lastLogIndex = 0
	} else {
		lastLogTerm = int64(server.log[len(server.log)-1].Term)
		lastLogIndex = int64(server.log[len(server.log)-1].Index)
	}
	return lastLogIndex, lastLogTerm
}

func (server *Server) retrievePrevLogIndexAndTerm(logIndex int) (int64, int64) {
	var prevLogTerm int64
	var prevLogIndex int64
	if logIndex == 0 {
		// If the servers log is empty we can use 0 for last log term and index, because all servers have these two
		// values set to 0 at the start, and they won't refuse the messages with these values
		prevLogTerm = 0
		prevLogIndex = 0
	} else {
		prevLogTerm = int64(server.log[logIndex-1].Term)
		prevLogIndex = int64(server.log[logIndex-1].Index)
	}
	return prevLogIndex, prevLogTerm
}

// Writing to log file

func (server *Server) writeToFile(writeString string) {
	_, err := server.file.WriteString(writeString)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
}

// Server log replication

func (server *Server) ClientRequest(ctx context.Context, in *sgrpc.ClientRequestMessage) (*sgrpc.ClientRequestResponse, error) {
	server.writeToFile("Received client request\n")

	server.clientRequestMutex.Lock()
	lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
	if int(lastLogTerm) != server.currentTerm {
		lastLogIndex = 0
	}
	newLog := log.Message{
		Term:  server.currentTerm,
		Index: int(lastLogIndex) + 1,
		Msg:   in.Message,
	}
	server.log = append(server.log, newLog)
	// Increase the number of logs replicated on this server
	server.matchIndex[0]++
	server.issueAppendEntries()
	server.clientRequestMutex.Unlock()
	return &sgrpc.ClientRequestResponse{Success: true}, nil
}

func (server *Server) issueAppendEntries() {
	prevLogIndex, prevLogTerm := server.retrievePrevLogIndexAndTerm(len(server.log) - 1)
	logLengthWhenIssuingAppendEntries := len(server.log)
	lastLog := server.log[logLengthWhenIssuingAppendEntries-1]
	appendEntryMessage := sgrpc.AppendEntryMessage{
		Term:          int64(server.currentTerm),
		LeaderAddress: server.serverAddress,
		PrevLogIndex:  prevLogIndex,
		PrevLogTerm:   prevLogTerm,
		Entry: &sgrpc.LogEntry{
			Term:    int64(lastLog.Term),
			Index:   int64(lastLog.Index),
			Message: lastLog.Msg,
		},
		LeaderCommit: int64(server.commitIndex),
	}
	for index, address := range server.serverAddresses {
		go func() {
			conn, err := grpc.NewClient(address, grpc.WithInsecure())
			if err != nil {
				panic(err)
			}
			defer conn.Close()
			grpcClient := sgrpc.NewServerServiceClient(conn)
			contextServer, cancel := context.WithTimeout(context.Background(), time.Millisecond*(electionTimeoutTime/8))
			defer cancel()

			appendEntryResponse, err := grpcClient.AppendEntry(contextServer, &appendEntryMessage)
			if err != nil {
				fmt.Println(err)
			} else {
				server.issueAppendEntryMutex.Lock()
				server.processAppendEntryResponse(appendEntryResponse, index, logLengthWhenIssuingAppendEntries)
				server.issueAppendEntryMutex.Unlock()
			}
		}()
	}
}

func (server *Server) processAppendEntryResponse(appendEntryResponse *sgrpc.AppendEntryResponse, serverIndex int, logLengthWhenIssuingAppendEntries int) {
	// Add 1, because the first index is reserved for the current server.
	if appendEntryResponse.Success {
		server.matchIndex[serverIndex+1]++
	}
	server.checkIfAppendEntryIsReplicatedOnMajorityOfServers(logLengthWhenIssuingAppendEntries)
}

func (server *Server) checkIfAppendEntryIsReplicatedOnMajorityOfServers(logLengthWhenIssuingAppendEntries int) {
	if server.log[logLengthWhenIssuingAppendEntries-1].Commited {
		return
	}
	numOfSuccessfulReplications := 0
	for _, value := range server.matchIndex {
		if value >= logLengthWhenIssuingAppendEntries {
			numOfSuccessfulReplications++
		}
	}
	if numOfSuccessfulReplications >= (len(server.matchIndex)+1)/2 {
		server.log[logLengthWhenIssuingAppendEntries-1].Commited = true
		server.commitAllPreviousEntries(logLengthWhenIssuingAppendEntries - 2)
	}
}

func (server *Server) commitAllPreviousEntries(startIndex int) {
	// startIndex points on log entry that is before the last one, when the appendEntry replication was issued.
	for !server.log[startIndex].Commited {
		server.log[startIndex].Commited = true
		startIndex--
	}
}
