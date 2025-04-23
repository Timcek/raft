package server

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"math"
	"math/rand"
	"net"
	"os"
	"raftImplementation/raft/log"
	sgrpc "raftImplementation/raft/server/src"
	"strconv"
	"sync"
	"time"
)

const LEADER = 1
const FOLLOWER = 2
const CANDIDATE = 3

const jsVisualizationSpeed = float64(1) / float64(100)

// This means the number of milliseconds. Current election timeout is set to 200ms, if we want to change that number we
// have to change it in visualization too.
var electionTimeoutTime = int(math.Pow(jsVisualizationSpeed, -1) * 200)

type Server struct {
	sgrpc.UnimplementedServerServiceServer
	currentTerm int
	// Tells us if this server already voted in currentTerm
	votedInThisTerm bool
	log             []log.Message
	// LEADER or FOLLOWER or CANDIDATE
	serverState int

	leaderAddress string
	// serverAddresses contains addresses of all the servers in the cluster
	serverAddresses []string
	// Current server address index in serverAddresses, used for other servers to set leader to this address, when this server is elected for leader
	serverAddressIndex int

	// Timers used for triggering elections and heartbeat
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer

	// Election parameters
	voteCount  int
	inElection bool

	// Locks needed for concurrent vote request and heartbeat sending
	electionMutex                   sync.Mutex
	heartbeatMutex                  sync.Mutex
	logReplicationMutex             sync.Mutex
	processAppendEntryResponseMutex sync.Mutex

	//volatile state on every server
	commitIndex int

	//volatile state on leader
	// Index 0 is reserved for current server and each of the other indexes is reserved for servers specified in serverAddresses
	nextIndex []int

	// Opened file for writing all the messages on the server. This enables us to easier keep track of what is going on in servers.
	file *os.File
}

// Server constructor

func CreateServer(addressIndex int, addresses []string) {
	// Create server with starting values
	server := Server{}
	server.currentTerm = 0
	server.votedInThisTerm = false
	server.serverState = FOLLOWER

	// We initialize commit index to -1, because the server log is empty and there is zero commited entries (when the
	// value is 0 this means that the first entry is commited)
	server.commitIndex = -1

	server.serverAddressIndex = addressIndex
	go startServerWebSocket(server.serverAddressIndex)
	time.Sleep(3 * time.Second)
	server.serverAddresses = addresses
	server.createElectionTimer()
	server.nextIndex = make([]int, len(server.serverAddresses))

	file, err := os.Create("output" + strconv.Itoa(addressIndex) + ".txt")
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
	listener, err := net.Listen("tcp", server.serverAddresses[server.serverAddressIndex])
	if err != nil {
		panic(err)
	}
	server.writeToFile("gRPC server listening at " + server.serverAddresses[server.serverAddressIndex] + "\n")
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
	server.writeToFile(fmt.Sprintf("%v", randomNum))
	server.electionTimer.Reset(time.Millisecond * time.Duration(randomNum))
	server.sendElectionTimeoutChange(randomNum)
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
	server.writeToFile("Election began " + server.serverAddresses[server.serverAddressIndex] + "\n")
}

func (server *Server) increaseTerm() {
	server.currentTerm = server.currentTerm + 1
	server.votedInThisTerm = false
	server.updateServerTerm()
}

func (server *Server) resetVoteCountAndVoteForYourself() {
	// Resets vote count to 0 and votes for himself. The vote count at the end is 1.
	server.voteCount = 1
	server.votedInThisTerm = true
}

func (server *Server) issueVoteRequestsToOtherServers() {
	fmt.Println("lkasjflksjf")
	for index, address := range server.serverAddresses {
		if index == server.serverAddressIndex {
			continue
		}
		go func() {
			conn, err := grpc.NewClient(address, grpc.WithInsecure())
			if err != nil {
				panic(err)
			}
			defer conn.Close()

			grpcClient := sgrpc.NewServerServiceClient(conn)

			server.writeToFile(fmt.Sprintf("timeout: %v", time.Millisecond*time.Duration(electionTimeoutTime/8)))
			contextServer, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(electionTimeoutTime/8))
			defer cancel()

			lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
			requestVote := sgrpc.RequestVoteMessage{
				Term:         int64(server.currentTerm),
				LastLogTerm:  lastLogTerm,
				LastLogIndex: lastLogIndex,
				ServerIndex:  int64(server.serverAddressIndex),
			}

			server.sendVoteRequest(index)
			time.Sleep(time.Duration(math.Pow(jsVisualizationSpeed, -1)) * 15 * time.Millisecond)
			voteResult, err := grpcClient.RequestVote(contextServer, &requestVote)
			time.Sleep(time.Duration(math.Pow(jsVisualizationSpeed, -1)) * 15 * time.Millisecond)
			if err != nil {
				server.writeToFile(fmt.Sprintf("%v", err))
			} else {
				server.processVoteResult(voteResult, index)
			}
		}()
	}
}

func (server *Server) processVoteResult(message *sgrpc.RequestVoteResponse, serverIndex int) {
	server.electionMutex.Lock()
	defer server.electionMutex.Unlock()
	if !server.inElection {
		return
	}
	if message.VoteGranted {
		server.grantVote(serverIndex)
		server.voteCount++
	} else {
		server.receivesDeniedVote(int(message.Term))
	}
	// We always use odd number of servers, so if the number of votes is greater than (serverAddresses+1)/2 (by adding
	// 1 we make sure, that the number is rounded up. For example if we use 5 servers and divide it in half we need to
	// round to 3 which represents majority) the majority has voted for this server to become the leader.
	if server.voteCount >= (len(server.serverAddresses)+1)/2 {
		// This server won the elections
		server.becomeLeader()
	}
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
	server.heartbeatTimer = time.NewTimer(time.Millisecond * time.Duration(electionTimeoutTime/4))
	go func() {
		for true {
			<-server.heartbeatTimer.C
			server.resetHeartbeat()
			server.heartbeatTimeout()
		}
	}()
}

func (server *Server) stopHeartbeat() {
	if server.heartbeatTimer != nil {
		server.heartbeatTimer.Stop()
	}
}

func (server *Server) resetHeartbeat() {
	// This function is different from the election timeout, because the election timout is first set when the server is
	// started but the heartbeat timeout is first set when the server becomes the leader for the first time
	if server.heartbeatTimer == nil {
		server.manageHeartbeat()
	} else {
		server.heartbeatTimer.Reset(time.Millisecond * time.Duration(electionTimeoutTime/4))
	}
}

func (server *Server) heartbeatTimeout() {
	server.writeToFile("Sending heartbeat " + fmt.Sprintf("%v\n", server.log))
	server.logReplicationMutex.Lock()
	lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
	var wg sync.WaitGroup
	for index, address := range server.serverAddresses {
		if index == server.serverAddressIndex {
			continue
		}
		if server.nextIndex[index] != server.nextIndex[server.serverAddressIndex] {
			server.writeToFile("Sending AppendEntry, index: \n" + fmt.Sprintf("%v", index) + ", nextIndex: " + fmt.Sprintf("%v", server.nextIndex))

			server.prepareAndSendAppendEntry(index, &wg, address)
		} else {
			server.prepareAndSendHeartbeat(lastLogIndex, lastLogTerm, address, index)
		}
	}
	wg.Wait()
	server.logReplicationMutex.Unlock()
}

func (server *Server) prepareAndSendAppendEntry(serverIndex int, wg *sync.WaitGroup, address string) {
	prevLogTerm, prevLogIndex := server.retrievePrevLogIndexAndTerm(server.nextIndex[serverIndex])

	message := sgrpc.AppendEntryMessage{
		Term:               int64(server.currentTerm),
		LeaderAddressIndex: int64(server.serverAddressIndex),
		PrevLogIndex:       prevLogTerm,
		PrevLogTerm:        prevLogIndex,
		Entry: &sgrpc.LogEntry{
			Term:     int64(server.log[server.nextIndex[serverIndex]].Term),
			Index:    int64(server.log[server.nextIndex[serverIndex]].Index),
			Message:  server.log[server.nextIndex[serverIndex]].Msg,
			Commited: server.log[server.nextIndex[serverIndex]].Commited,
		},
		LeaderCommit: int64(server.commitIndex),
	}
	// serverLogLength represents the address server's log length after appending the message.
	// This is the length for which the majority replication should be checked for.
	serverLogLength := server.nextIndex[serverIndex] + 1
	wg.Add(1)
	server.sendAppendEntryMessage(address, &message, serverIndex, serverLogLength, wg)
}

func (server *Server) prepareAndSendHeartbeat(lastLogIndex int64, lastLogTerm int64, address string, serverIndex int) {
	message := sgrpc.AppendEntryMessage{
		Term:               int64(server.currentTerm),
		LeaderAddressIndex: int64(server.serverAddressIndex),
		PrevLogIndex:       lastLogIndex,
		PrevLogTerm:        lastLogTerm,
		Entry:              nil,
		LeaderCommit:       int64(server.commitIndex),
	}
	server.sendHeartbeatMessage(address, &message, nil, serverIndex)
}

// Change server state

func (server *Server) becomeCandidate() {
	server.inElection = false
	server.serverState = CANDIDATE
	server.stopHeartbeat()
	server.resetElectionTimer()
}

func (server *Server) becomeFollower(leaderAddressIndex int) {
	server.inElection = false
	server.serverState = FOLLOWER
	server.stopHeartbeat()
	server.resetElectionTimer()
	server.leaderAddress = server.serverAddresses[leaderAddressIndex]
}

func (server *Server) becomeLeader() {
	server.serverState = LEADER
	server.inElection = false
	server.initializeNextIndex()
	server.stopElectionTimer()
	server.sendBecomeLeader()
	// The first heartbeat after election shouldn't send out appendEntries, so we can adjust matchIndex for every server
	server.heartbeatTimeout()
	server.resetHeartbeat()
}

func (server *Server) initializeNextIndex() {
	logLength := len(server.log)
	// Initialize nextIndex array to the length of the leaders log. If the replicated logs on other servers are not the
	// same, they will get fixed with appendEntries and heartbeats
	for index, _ := range server.nextIndex {
		server.nextIndex[index] = logLength
	}
}

// Receive and respond to AppendEntry

func (server *Server) AppendEntry(ctx context.Context, in *sgrpc.AppendEntryMessage) (*sgrpc.AppendEntryResponse, error) {
	if int(in.Term) < server.currentTerm {
		return server.receivesHeartbeatOrAppendEntryWithStaleTerm(), nil
	}
	server.replaceServersCurrentTermIfReceivedTermInHeartbeatIsHigher(int(in.Term))

	if in.Entry == nil {
		server.writeToFile(" Received heartbeat " + fmt.Sprintf("%v", server.log) + "\n")
		server.becomeFollower(int(in.LeaderAddressIndex))
		return server.checkTheReceivedHeartbeat(in)
	} else if len(server.log) != 0 && server.log[len(server.log)-1].Term == int(in.Entry.Term) &&
		server.log[len(server.log)-1].Index == int(in.Entry.Index) {
		// Received append entry with the same entry as the last log entry
		server.becomeFollower(int(in.LeaderAddressIndex))
		return &sgrpc.AppendEntryResponse{
			Term:    int64(server.currentTerm),
			Success: true,
		}, nil
	} else if (len(server.log) == 0 && int(in.PrevLogTerm) == 0 && int(in.PrevLogIndex) == 0) ||
		(len(server.log) != 0 && (int(in.PrevLogTerm) == server.log[len(server.log)-1].Term && int(in.PrevLogIndex) == server.log[len(server.log)-1].Index)) {
		server.writeToFile("Received append entry \n" + fmt.Sprintf("%v", in))
		// AppendEntry is valid, if previous log term and index equal the last log entry, or they are 0,
		// which means that this should be the first entry in log
		server.becomeFollower(int(in.LeaderAddressIndex))
		return server.receivedValidAppendEntry(in), nil
	} else {
		// Received AppendEntry does not fit on the end of the log. We start moving backwards towards the commited part
		// of the log to find where the entry can be inserted. If we reach the commited part of the log before finding
		// the position we return success=false and the leader will keep on sending his previous log entries until we
		// find log entry that fits into our log. The worst case scenario is that we move back all the way to the
		// commited part and have to replace all log entries after the commited part.
		if in.PrevLogTerm == 0 && in.PrevLogIndex == 0 {
			return server.receivedFirstLogEntryButCurrentServerLogIsNotEmpty(in)
		}
		return server.findLogPositionAndInsertLogEntry(in, int(in.LeaderCommit))
	}
}

func (server *Server) receivesHeartbeatOrAppendEntryWithStaleTerm() *sgrpc.AppendEntryResponse {
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: false,
	}
}

func (server *Server) replaceServersCurrentTermIfReceivedTermInHeartbeatIsHigher(term int) {
	if term > server.currentTerm {
		server.changeTerm(term, false)
	}
}

func (server *Server) checkTheReceivedHeartbeat(in *sgrpc.AppendEntryMessage) (*sgrpc.AppendEntryResponse, error) {
	if in.PrevLogTerm != 0 && in.PrevLogIndex != 0 &&
		(len(server.log) == 0 ||
			server.log[len(server.log)-1].Term != int(in.PrevLogTerm) ||
			server.log[len(server.log)-1].Index != int(in.PrevLogIndex)) {
		return server.receivedHeartbeatWithNewerLog(int(in.LeaderAddressIndex)), nil
	}
	server.commitEntriesOnFollower(int(in.LeaderCommit))
	return server.receivedValidHeartbeat(int(in.LeaderAddressIndex)), nil
}

func (server *Server) receivedHeartbeatWithNewerLog(leaderAddressIndex int) *sgrpc.AppendEntryResponse {
	server.sendHeartbeatReply(leaderAddressIndex, false)
	time.Sleep(time.Duration(math.Pow(jsVisualizationSpeed, -1)) * 15 * time.Millisecond)
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: false,
	}
}

func (server *Server) receivedValidHeartbeat(leaderAddressIndex int) *sgrpc.AppendEntryResponse {
	server.sendHeartbeatReply(leaderAddressIndex, true)
	time.Sleep(time.Duration(math.Pow(jsVisualizationSpeed, -1)) * 15 * time.Millisecond)
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: true,
	}
}

func (server *Server) receivedValidAppendEntry(in *sgrpc.AppendEntryMessage) *sgrpc.AppendEntryResponse {
	server.resetElectionTimer()
	server.appendToLog(in.Entry)
	server.commitEntriesOnFollower(int(in.LeaderCommit))
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: true,
	}
}

func (server *Server) receivedFirstLogEntryButCurrentServerLogIsNotEmpty(in *sgrpc.AppendEntryMessage) (*sgrpc.AppendEntryResponse, error) {
	if !server.log[0].Commited {
		server.becomeFollower(int(in.LeaderAddressIndex))
		server.log = server.log[:0]
		server.appendToLog(in.Entry)
		server.commitEntriesOnFollower(int(in.LeaderCommit))
		return &sgrpc.AppendEntryResponse{
			Term:    int64(server.currentTerm),
			Success: true,
		}, nil
	}
	server.electionTimeout()
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: false,
	}, nil
}

func (server *Server) findLogPositionAndInsertLogEntry(in *sgrpc.AppendEntryMessage, commitIndex int) (*sgrpc.AppendEntryResponse, error) {
	position := len(server.log) - 1
	for position != -1 && !server.log[position].Commited {
		if server.log[position-1].Term == int(in.PrevLogTerm) && server.log[position-1].Index == int(in.PrevLogIndex) {
			server.log = server.log[:position]
			server.appendToLog(in.Entry)
			server.becomeFollower(int(in.LeaderAddressIndex))
			server.commitEntriesOnFollower(commitIndex)
			return &sgrpc.AppendEntryResponse{
				Term:    int64(server.currentTerm),
				Success: true,
			}, nil
		}
		position--
	}
	if position != -1 && in.Entry.Commited && (server.log[position].Term > int(in.Entry.Term) ||
		server.log[position].Term == int(in.Entry.Term) && server.log[position].Index > int(in.Entry.Index)) {
		//We received append entry that is already in the commited part of the log (our log is newer)
		server.electionTimeout()
		return &sgrpc.AppendEntryResponse{
			Term:    int64(server.currentTerm),
			Success: false,
		}, nil
	}
	//We received commited append entry that is not in our log
	server.becomeFollower(int(in.LeaderAddressIndex))
	// We can not insert log entry into log, because the last log entries do not match with leaders. Leader has to send
	// us previous log entries for us to fix our log.
	return &sgrpc.AppendEntryResponse{
		Term:    int64(server.currentTerm),
		Success: false,
	}, nil
}

func (server *Server) commitEntriesOnFollower(commitIndex int) {
	if len(server.log) == 0 {
		// There are no entries that could be commited
		return
	}
	if len(server.log) <= commitIndex {
		server.commitAllEntriesUpToCommitIndex(len(server.log) - 1)
	} else {
		server.commitAllEntriesUpToCommitIndex(commitIndex)
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
		server.sendVoteReply(int(in.ServerIndex), false)
		return &sgrpc.RequestVoteResponse{
			Term:        int64(server.currentTerm),
			VoteGranted: false,
		}, nil
	}

	// Voters log is more up-to-date than the candidates log
	if int(in.LastLogTerm) < lastLog.Term || (int(in.LastLogTerm) == lastLog.Term && int(in.LastLogIndex) < lastLog.Index) {
		server.sendVoteReply(int(in.ServerIndex), false)
		return &sgrpc.RequestVoteResponse{
			Term:        int64(server.currentTerm),
			VoteGranted: false,
		}, nil
	}

	// Voter already voted in this term
	if server.votedInThisTerm {
		server.sendVoteReply(int(in.ServerIndex), false)
		return &sgrpc.RequestVoteResponse{
			Term:        int64(server.currentTerm),
			VoteGranted: false,
		}, nil
	}

	//Voter votes for the candidate
	server.resetElectionTimer()
	server.becomeCandidate()
	server.changeTerm(int(in.Term), true)
	server.sendVoteReply(int(in.ServerIndex), true)
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
	server.updateServerTerm()
}

// Retrieve log info

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

// Server log management

func (server *Server) appendToLog(newLogEntry *sgrpc.LogEntry) {
	server.log = append(server.log, log.Message{
		Term:     int(newLogEntry.Term),
		Index:    int(newLogEntry.Index),
		Msg:      newLogEntry.Message,
		Commited: false,
	})
}

func (server *Server) commitAllEntriesUpToCommitIndex(commitIndex int) {
	// startIndex points on log entry that is before the last one, when the appendEntry replication was issued.
	server.commitIndex = commitIndex
	for commitIndex >= 0 && !server.log[commitIndex].Commited {
		server.log[commitIndex].Commited = true
		commitIndex--
	}
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

// ClientRequest is used for adding log entries to servers log.
//
// If this server is not the leader it returns Success=false and LeaderAddress contains address of the real leader.
// The client that called this method should send another request with the same log entry to the received LeaderAddress.
func (server *Server) ClientRequest(ctx context.Context, in *sgrpc.ClientRequestMessage) (*sgrpc.ClientRequestResponse, error) {
	if server.serverState != LEADER {
		return &sgrpc.ClientRequestResponse{
			Success:       false,
			LeaderAddress: server.leaderAddress,
		}, nil
	}
	server.writeToFile("Received client request\n")

	server.logReplicationMutex.Lock()
	lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
	if int(lastLogTerm) != server.currentTerm {
		lastLogIndex = 0
	}
	newLog := sgrpc.LogEntry{
		Term:    int64(server.currentTerm),
		Index:   int64(lastLogIndex) + 1,
		Message: in.Message,
	}
	server.appendToLog(&newLog)
	// Increase the number of logs replicated on this server
	server.nextIndex[server.serverAddressIndex]++

	server.resetHeartbeat()
	server.sendAppendEntries()
	server.logReplicationMutex.Unlock()

	return &sgrpc.ClientRequestResponse{Success: true}, nil
}

func (server *Server) sendAppendEntries() {
	prevLogIndex, prevLogTerm := server.retrievePrevLogIndexAndTerm(len(server.log) - 1)
	logLengthWhenIssuingAppendEntries := len(server.log)
	lastLog := server.log[logLengthWhenIssuingAppendEntries-1]
	appendEntryMessage := sgrpc.AppendEntryMessage{
		Term:               int64(server.currentTerm),
		LeaderAddressIndex: int64(server.serverAddressIndex),
		PrevLogIndex:       prevLogIndex,
		PrevLogTerm:        prevLogTerm,
		Entry: &sgrpc.LogEntry{
			Term:    int64(lastLog.Term),
			Index:   int64(lastLog.Index),
			Message: lastLog.Msg,
		},
		LeaderCommit: int64(server.commitIndex),
	}

	var wg sync.WaitGroup
	for index, address := range server.serverAddresses {
		if index == server.serverAddressIndex {
			continue
		}
		// If the number of log entries on the current server is for one bigger than the other server,
		// then we can send append entries, otherwise they will be sent with heartbeats
		if server.nextIndex[index] == logLengthWhenIssuingAppendEntries-1 {
			wg.Add(1)
			server.sendAppendEntryMessage(address, &appendEntryMessage, index, logLengthWhenIssuingAppendEntries, &wg)
		}
	}
	// We wait for all the AppendEntry messages to return, only then we allow new AppendEntry to be sent.
	wg.Wait()
}

// Messages sending

func (server *Server) sendHeartbeatMessage(address string, heartbeatMessage *sgrpc.AppendEntryMessage,
	waitGroup *sync.WaitGroup, serverIndex int) {
	go func() {
		if waitGroup != nil {
			defer waitGroup.Done()
		}
		conn, err := grpc.NewClient(address, grpc.WithInsecure())
		if err != nil {
			panic(err)
		}
		defer conn.Close()

		grpcClient := sgrpc.NewServerServiceClient(conn)

		contextServer, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(electionTimeoutTime/8))
		defer cancel()

		server.sendHeartbeat(serverIndex)
		time.Sleep(time.Duration(math.Pow(jsVisualizationSpeed, -1)) * 15 * time.Millisecond)
		heartbeatResponse, err := grpcClient.AppendEntry(contextServer, heartbeatMessage)
		if err != nil {
			fmt.Println(err)
		} else {
			server.heartbeatMutex.Lock()
			server.processHeartbeatResponse(heartbeatResponse, serverIndex)
			server.heartbeatMutex.Unlock()
		}
	}()
}

func (server *Server) processHeartbeatResponse(response *sgrpc.AppendEntryResponse, serverIndex int) {
	if !response.Success && int(response.Term) > server.currentTerm {
		// We receive success false, because the other server has higher term tha this
		server.becomeCandidate()
		server.changeTerm(int(response.Term), false)
	} else if !response.Success {
		// We receive success false, because this leader has different log than the follower, to which the appendEntry was sent.
		server.nextIndex[serverIndex]--
	}
}

func (server *Server) sendAppendEntryMessage(address string, appendEntryMessage *sgrpc.AppendEntryMessage, serverIndex int,
	logLengthToCheckForMajorityReplication int, waitGroup *sync.WaitGroup) {
	go func() {
		if waitGroup != nil {
			defer waitGroup.Done()
		}
		conn, err := grpc.NewClient(address, grpc.WithInsecure())
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		grpcClient := sgrpc.NewServerServiceClient(conn)
		contextServer, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(electionTimeoutTime/8))
		defer cancel()

		server.sendAppendEntry(serverIndex)
		appendEntryResponse, err := grpcClient.AppendEntry(contextServer, appendEntryMessage)
		if err != nil {
			fmt.Println(err)
		} else {
			server.processAppendEntryResponseMutex.Lock()
			server.processAppendEntryResponse(appendEntryResponse, serverIndex, logLengthToCheckForMajorityReplication)
			server.processAppendEntryResponseMutex.Unlock()
		}
	}()
}

func (server *Server) processAppendEntryResponse(appendEntryResponse *sgrpc.AppendEntryResponse, serverIndex int,
	logLengthToCheckForMajorityReplication int) {
	// Add 1, because the first index is reserved for the current server.
	server.writeToFile("AppendEntryResponse " + fmt.Sprintf("%v\n", appendEntryResponse))
	if appendEntryResponse.Success && server.nextIndex[serverIndex] < server.nextIndex[server.serverAddressIndex] {
		server.nextIndex[serverIndex]++
		server.checkIfAppendEntryIsReplicatedOnMajorityOfServers(logLengthToCheckForMajorityReplication)
	} else if !appendEntryResponse.Success && int(appendEntryResponse.Term) > server.currentTerm {
		// We receive success=false, because the other server has higher term
		server.becomeCandidate()
		server.changeTerm(int(appendEntryResponse.Term), false)
	} else if !appendEntryResponse.Success {
		// We receive success=false, because this leader has different log than the follower, to which the appendEntry was sent.
		server.nextIndex[serverIndex]--
	}
}

func (server *Server) checkIfAppendEntryIsReplicatedOnMajorityOfServers(logLengthToCheckForMajorityReplication int) {
	if server.log[logLengthToCheckForMajorityReplication-1].Commited ||
		server.log[logLengthToCheckForMajorityReplication-1].Term != server.currentTerm {
		return
	}
	numOfSuccessfulReplications := 0
	for _, value := range server.nextIndex {
		if value >= logLengthToCheckForMajorityReplication {
			numOfSuccessfulReplications++
		}
	}
	if numOfSuccessfulReplications >= (len(server.nextIndex)+1)/2 {
		server.commitAllEntriesUpToCommitIndex(logLengthToCheckForMajorityReplication - 1)
	}
}

//TODO potrebno implementirati, da se sproži takojšnje proženje pošiljanje novega append entry ne da se čaka na heartbeat timeout
