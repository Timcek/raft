package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"fmt"
	"math/rand"
	"os"
	"raftImplementation/raft/log"
	sgrpc "raftImplementation/raft/server/src"
	"sync"
	"time"
)
import "raftImplementation/princeton/assignment3/src/labrpc"

// import "bytes"
// import "encoding/gob"
const LEADER = 1
const FOLLOWER = 2
const CANDIDATE = 3

// This means the number of milliseconds
const electionTimeoutTime = 1000

type Raft struct {
	sgrpc.UnimplementedServerServiceServer

	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	applyCh   chan ApplyMsg

	me int // index into peers[]

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
	electionMutex                   sync.Mutex
	heartbeatMutex                  sync.Mutex
	logReplicationMutex             sync.Mutex
	processAppendEntryResponseMutex sync.Mutex
	writeToFileMutex                sync.Mutex

	//volatile state on every server
	commitIndex int

	//volatile state on leader
	// Index 0 is reserved for current server and each of the other indexes is reserved for servers specified in serverAddresses
	nextIndex []int

	// Opened file for writing all the messages on the server. This enables us to easier keep track of what is going on in servers.
	file *os.File
}

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

// example RequestVote RPC arguments structure.
type AppendEntryArgs struct {
	Term          int
	LeaderAddress string
	PrevLogIndex  int
	PrevLogTerm   int
	Entry         log.Message
	LeaderCommit  int
}

// example RequestVote RPC reply structure.
type AppendEntryReply struct {
	Term    int
	Success bool
}

type RequestVoteArgs struct {
	Term         int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type ClientRequestArgs struct {
	Message interface{}
}

// example RequestVote RPC reply structure.
type ClientRequestReply struct {
	Success  bool
	Position int
}

// Server constructor

func CreateServer(peers []*labrpc.ClientEnd, me int, persister *Persister, applyCh chan ApplyMsg) *Raft {
	// Create server with starting values
	server := Raft{}

	server.peers = peers
	server.persister = persister
	server.me = me
	server.applyCh = applyCh

	server.currentTerm = 0
	server.votedInThisTerm = false
	server.serverState = FOLLOWER

	go func() {
		file, err := os.Create("output" + fmt.Sprintf("%v", server.me) + time.Now().String() + ".txt")
		defer file.Close()
		if err != nil {
			fmt.Println("Error creating file:", err)
			return
		}
		server.file = file
		time.Sleep(10000 * time.Second)
	}()

	// We initialize commit index to -1, because the server log is empty and there is zero commited entries (when the
	// value is 0 this means that the first entry is commited)
	server.commitIndex = -1
	server.createElectionTimer()
	server.nextIndex = make([]int, len(server.peers))

	return &server
}

// Election timer management. When the timer triggers the election process starts.

func (server *Raft) createElectionTimer() {
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

func (server *Raft) resetElectionTimer() {
	randomNum := rand.Intn(electionTimeoutTime/2) + electionTimeoutTime/2
	server.electionTimer.Reset(time.Millisecond * time.Duration(randomNum))
}

func (server *Raft) stopElectionTimer() {
	if server.electionTimer != nil {
		server.electionTimer.Stop()
	}
}

func (server *Raft) electionTimeout() {
	server.becomeCandidate()
	server.beginElection()
}

// Begin election process, send votes, wait for responses and change server state based on vote result.

func (server *Raft) beginElection() {
	server.increaseTerm()
	server.inElection = true
	server.resetVoteCountAndVoteForYourself()
	server.issueVoteRequestsToOtherServers()
	server.resetElectionTimer()
	server.writeToFile(time.Now().String() + " Election began " + fmt.Sprintf("%v\n", server.me))
}

func (server *Raft) increaseTerm() {
	server.currentTerm = server.currentTerm + 1
	server.votedInThisTerm = false
}

func (server *Raft) resetVoteCountAndVoteForYourself() {
	// Resets vote count to 0 and votes for himself. The vote count at the end is 1.
	server.voteCount = 1
	server.votedInThisTerm = true
}

func (server *Raft) issueVoteRequestsToOtherServers() {
	for index, _ := range server.peers {
		if index == server.me {
			continue
		}
		go func() {
			lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
			requestVote := RequestVoteArgs{
				Term:         server.currentTerm,
				LastLogTerm:  lastLogTerm,
				LastLogIndex: lastLogIndex,
			}
			voteResult := RequestVoteReply{}
			ok := server.peers[index].Call("Raft.RequestVote", requestVote, &voteResult, 100)
			if !ok {
				server.writeToFile("Problem with sending vote requests\n")
			} else {
				server.processVoteResult(&voteResult)
			}
		}()
	}
}

func (server *Raft) processVoteResult(message *RequestVoteReply) {
	server.writeToFile("Processing vote result")
	server.electionMutex.Lock()
	defer server.electionMutex.Unlock()
	if !server.inElection {
		server.writeToFile("Not in election")
		return
	}
	if message.VoteGranted {
		server.voteCount++
	} else {
		server.receivesDeniedVote(message.Term)
	}
	server.writeToFile(fmt.Sprintf("Vote count: %v\n", server.voteCount))
	// We always use odd number sof servers so if the number of votes is greater than serverAddresses/2 the majority has
	// voted for this server to become a leader.
	// We add 1 so that the number is ceil rounded
	if server.voteCount >= (len(server.peers)+1)/2 {
		// This server won the elections
		server.becomeLeader()
	}
}

func (server *Raft) receivesDeniedVote(term int) {
	// if the term received in a message is higher than the server's current term we have to update it and stop elections.
	if term > server.currentTerm {
		server.changeTerm(term, false)
		server.inElection = false
		server.resetElectionTimer()
	}
}

// Leader heartbeat management

func (server *Raft) manageHeartbeat() {
	server.heartbeatTimer = time.NewTimer(time.Millisecond * (electionTimeoutTime / 4))
	go func() {
		for true {
			<-server.heartbeatTimer.C
			server.resetHeartbeat()
			server.heartbeatTimeout()
		}
	}()
}

func (server *Raft) stopHeartbeat() {
	if server.heartbeatTimer != nil {
		server.heartbeatTimer.Stop()
	}
}

func (server *Raft) resetHeartbeat() {
	// This function is different from the election timeout, because the election timout is first set when the server is
	// started but the heartbeat timeout is first set when the server becomes the leader for the first time
	if server.heartbeatTimer == nil {
		server.manageHeartbeat()
	} else {
		server.heartbeatTimer.Reset(time.Millisecond * (electionTimeoutTime / 4))
	}
}

func (server *Raft) heartbeatTimeout() {
	server.writeToFile(time.Now().String() + " Sending heartbeat " + fmt.Sprintf("%v\n", server.log))
	server.logReplicationMutex.Lock()
	lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
	var wg sync.WaitGroup
	for index, _ := range server.nextIndex {
		server.writeToFile("tttttttttttttttttttttttt\n")
		if index == server.me {
			continue
		}
		if server.nextIndex[index] != server.nextIndex[server.me] {
			server.writeToFile("Sending append entry from heartbeat\n")
			server.writeToFile("Sending AppendEntry, index: \n" + fmt.Sprintf("%v", index) + ", nextIndex: " + fmt.Sprintf("%v", server.nextIndex))
			server.prepareAndSendAppendEntry(index, &wg)
			server.writeToFile("-----------------------------llllllllllllllllllllll\n")
		} else {
			server.prepareAndSendHeartbeat(lastLogIndex, lastLogTerm, index)
		}
	}
	wg.Wait()
	server.logReplicationMutex.Unlock()
}

func (server *Raft) prepareAndSendAppendEntry(index int, wg *sync.WaitGroup) {
	prevLogTerm, prevLogIndex := server.retrievePrevLogIndexAndTerm(server.nextIndex[index])

	message := AppendEntryArgs{
		Term:         server.currentTerm,
		PrevLogIndex: prevLogTerm,
		PrevLogTerm:  prevLogIndex,
		Entry: log.Message{
			Term:     server.log[server.nextIndex[index]].Term,
			Index:    server.log[server.nextIndex[index]].Index,
			Msg:      server.log[server.nextIndex[index]].Msg,
			Commited: server.log[server.nextIndex[index]].Commited,
		},
		LeaderCommit: server.commitIndex,
	}
	// serverLogLength represents the address server's log length after appending the message.
	// This is the length for which the majority replication should be checked for.
	serverLogLength := server.nextIndex[index] + 1
	wg.Add(1)
	server.sendAppendEntryMessage(message, index, serverLogLength, wg)
}

func (server *Raft) prepareAndSendHeartbeat(lastLogIndex int, lastLogTerm int, serverIndex int) {
	message := AppendEntryArgs{
		Term:         server.currentTerm,
		PrevLogIndex: lastLogIndex,
		PrevLogTerm:  lastLogTerm,
		Entry:        log.Message{},
		LeaderCommit: server.commitIndex,
	}
	server.sendHeartbeatMessage(message, serverIndex, nil)
}

// Change server state

func (server *Raft) becomeCandidate() {
	server.inElection = false
	server.serverState = CANDIDATE
	server.writeToFile(time.Now().String() + " stopping heartbeat\n")
	server.stopHeartbeat()
	server.resetElectionTimer()
}

func (server *Raft) becomeFollower(leaderAddress string) {
	server.inElection = false
	server.serverState = FOLLOWER
	server.stopHeartbeat()
	server.resetElectionTimer()
	server.leaderAddress = leaderAddress
}

func (server *Raft) becomeLeader() {
	server.serverState = LEADER
	server.inElection = false
	server.initializeNextIndex()
	server.writeToFile("leader prvič\n")
	server.stopElectionTimer()
	server.writeToFile("leader drugič\n")
	// The first heartbeat after election shouldn't send out appendEntries, so we can adjust matchIndex for every server
	server.heartbeatTimeout()
	server.writeToFile("leader tretjič\n")
	server.resetHeartbeat()
}

func (server *Raft) initializeNextIndex() {
	logLength := len(server.log)
	// Initialize nextIndex array to the length of the leaders log. If the replicated logs on other servers are not the
	// same, they will get fixed with appendEntries and heartbeats
	for index, _ := range server.nextIndex {
		server.nextIndex[index] = logLength
	}
}

// Receive and respond to AppendEntry

func (server *Raft) AppendEntry(in AppendEntryArgs, out *AppendEntryReply) {
	if in.Term < server.currentTerm {
		server.receivesHeartbeatOrAppendEntryWithStaleTerm(out)
	}
	server.replaceServersCurrentTermIfReceivedTermInHeartbeatIsHigher(in.Term)

	if in.Entry == (log.Message{}) {
		server.writeToFile(time.Now().String() + " Received heartbeat " + fmt.Sprintf("%v", server.log) + "\n")
		server.becomeFollower(in.LeaderAddress)
		server.checkTheReceivedHeartbeat(in, out, in.LeaderCommit)
	} else if len(server.log) != 0 && server.log[len(server.log)-1].Term == int(in.Entry.Term) &&
		server.log[len(server.log)-1].Index == in.Entry.Index {
		// Received append entry with the same entry as the last log entry
		server.becomeFollower(in.LeaderAddress)
		out.Term = server.currentTerm
		out.Success = true
	} else if (len(server.log) == 0 && int(in.PrevLogTerm) == 0 && int(in.PrevLogIndex) == 0) ||
		(len(server.log) != 0 && (in.PrevLogTerm == server.log[len(server.log)-1].Term && in.PrevLogIndex == server.log[len(server.log)-1].Index)) {
		server.writeToFile(time.Now().String() + " Received append entry " + fmt.Sprintf("%v\n", in))
		// AppendEntry is valid, if previous log term and index equal the last log entry, or they are 0,
		// which means that this should be the first entry in log
		server.becomeFollower(in.LeaderAddress)
		server.receivedValidAppendEntry(&in.Entry, out, in.LeaderCommit)
	} else {
		// Received AppendEntry does not fit on the end of the log. We start moving backwards towards the commited part
		// of the log to find where the entry can be inserted. If we reach the commited part of the log before finding
		// the position we return success=false and the leader will keep on sending his previous log entries until we
		// find log entry that fits into our log. The worst case scenario is that we move back all the way to the
		// commited part and have to replace all log entries after the commited part.
		server.writeToFile(time.Now().String() + " Received append entry second " + fmt.Sprintf("%v\n", in))
		if in.PrevLogTerm == 0 && in.PrevLogIndex == 0 {
			server.receivedFirstLogEntryButCurrentServerLogIsNotEmpty(&in, out, in.LeaderCommit)
			return
		}
		server.findLogPositionAndInsertLogEntry(&in, out, in.LeaderCommit)
	}
}

func (server *Raft) receivesHeartbeatOrAppendEntryWithStaleTerm(out *AppendEntryReply) {
	out.Term = server.currentTerm
	out.Success = false
}

func (server *Raft) replaceServersCurrentTermIfReceivedTermInHeartbeatIsHigher(term int) {
	if term > server.currentTerm {
		server.changeTerm(term, false)
	}
}

func (server *Raft) checkTheReceivedHeartbeat(in AppendEntryArgs, out *AppendEntryReply, commitIndex int) {
	if in.PrevLogTerm != 0 && in.PrevLogIndex != 0 &&
		(len(server.log) == 0 ||
			server.log[len(server.log)-1].Term != in.PrevLogTerm ||
			server.log[len(server.log)-1].Index != in.PrevLogIndex) {
		server.receivedHeartbeatWithNewerLog(out)
		return
	}
	server.commitEntriesOnFollower(commitIndex)
	server.receivedValidHeartbeat(out)
}

func (server *Raft) receivedHeartbeatWithNewerLog(out *AppendEntryReply) {
	out.Term = server.currentTerm
	out.Success = false
}

func (server *Raft) receivedValidHeartbeat(out *AppendEntryReply) {
	out.Term = server.currentTerm
	out.Success = true
}

func (server *Raft) receivedValidAppendEntry(newLogEntry *log.Message, out *AppendEntryReply, commitIndex int) {
	server.resetElectionTimer()
	server.appendToLog(newLogEntry)
	server.commitEntriesOnFollower(commitIndex)
	out.Term = server.currentTerm
	out.Success = true
}

func (server *Raft) receivedFirstLogEntryButCurrentServerLogIsNotEmpty(in *AppendEntryArgs, out *AppendEntryReply, commitIndex int) {
	out.Term = server.currentTerm
	if !server.log[0].Commited {
		server.becomeFollower(in.LeaderAddress)
		server.writeToFile("receivedFirstLogEntryButCurrentServerLogIsNotEmpty\n")
		server.log = server.log[:0]
		server.appendToLog(&in.Entry)
		server.commitEntriesOnFollower(commitIndex)
		out.Success = true
		return
	}
	server.electionTimeout()
	// We send back true, because if we send back false it will cause an error, since it will lower next index to -1
	out.Success = true
}

func (server *Raft) findLogPositionAndInsertLogEntry(in *AppendEntryArgs, out *AppendEntryReply, commitIndex int) {
	position := len(server.log) - 1
	for position != -1 && !server.log[position].Commited {
		if server.log[position-1].Term == in.PrevLogTerm && server.log[position-1].Index == in.PrevLogIndex {
			server.writeToFile("findLogPositionAndInsertLogEntry\n")
			server.log = server.log[:position]
			server.appendToLog(&in.Entry)
			server.becomeFollower(in.LeaderAddress)
			server.commitEntriesOnFollower(commitIndex)
			return &sgrpc.AppendEntryResponse{
				Term:    int64(server.currentTerm),
				Success: true,
			}, nil
		}
		position--
	}
	if position != -1 && in.Entry.Commited && (server.log[position].Term > in.Entry.Term ||
		server.log[position].Term == in.Entry.Term && server.log[position].Index > in.Entry.Index) {
		//We received append entry that is already in the commited part of the log (our log is newer)
		out.Term = server.currentTerm
		out.Success = false
		server.electionTimeout()
		return
	}
	//We received commited append entry that is not in our log
	server.becomeFollower(in.LeaderAddress)
	// We can not insert log entry into log, because the last log entries do not match with leaders. Leader has to send
	// us previous log entries for us to fix our log.
	out.Term = server.currentTerm
	out.Success = false
}

func (server *Raft) commitEntriesOnFollower(commitIndex int) {
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

func (server *Raft) RequestVote(in RequestVoteArgs, out *RequestVoteReply) {
	server.writeToFile(time.Now().String() + " Received vote request\n")
	server.replaceServersCurrentTermIfReceivedTermInRequestVoteIsHigher(in.Term)
	server.writeToFile(fmt.Sprintf("%v\n", server.log))
	// Retrieve servers last log message (if the log is empty, the term and index should be 0, which they are by default when we create logMessage)
	var lastLog log.Message
	if len(server.log) > 0 {
		server.writeToFile(time.Now().String() + " prvi " + fmt.Sprintf("%v\n", len(server.log)))
		lastLog = server.log[len(server.log)-1]
	}
	server.writeToFile(fmt.Sprintf("current term: %v\n", server.currentTerm))
	server.writeToFile(fmt.Sprintf("%v\n", in))
	server.writeToFile(fmt.Sprintf("%v\n", lastLog))

	// Voters term is higher than candidates
	if in.Term < server.currentTerm {
		server.writeToFile(time.Now().String() + " drug\n")
		out.Term = server.currentTerm
		out.VoteGranted = false
		return
	}

	// Voters log is more up-to-date than the candidates log
	if in.LastLogTerm < lastLog.Term || (in.LastLogTerm == lastLog.Term && in.LastLogIndex < lastLog.Index) {
		server.writeToFile(time.Now().String() + " tretji\n")
		out.Term = server.currentTerm
		out.VoteGranted = false
		return
	}

	// Voter already voted in this term
	if server.votedInThisTerm {
		server.writeToFile(time.Now().String() + " cetrti\n")
		out.Term = server.currentTerm
		out.VoteGranted = false
		return
	}
	server.writeToFile(time.Now().String() + " peti\n")
	//Voter votes for the candidate
	server.resetElectionTimer()
	server.becomeCandidate()
	server.changeTerm(in.Term, true)
	out.Term = server.currentTerm
	out.VoteGranted = true
	return
}

func (server *Raft) replaceServersCurrentTermIfReceivedTermInRequestVoteIsHigher(term int) {
	if term > server.currentTerm {
		server.becomeCandidate()
		server.changeTerm(term, false)
	}
}

// Change term

func (server *Raft) changeTerm(term int, votedInThisTerm bool) {
	server.currentTerm = term
	server.votedInThisTerm = votedInThisTerm
}

// Retrieve log info

func (server *Raft) retrieveLastLogIndexAndTerm() (int, int) {
	var lastLogTerm int
	var lastLogIndex int
	if len(server.log) == 0 {
		// If the servers log is empty we can use 0 for last log term and index, because all servers have these two
		// values set to 0 at the start, and they won't refuse the messages with these values
		lastLogTerm = 0
		lastLogIndex = 0
	} else {
		lastLogTerm = server.log[len(server.log)-1].Term
		lastLogIndex = server.log[len(server.log)-1].Index
	}
	return lastLogIndex, lastLogTerm
}

func (server *Raft) retrievePrevLogIndexAndTerm(logIndex int) (int, int) {
	var prevLogTerm int
	var prevLogIndex int
	if logIndex == 0 {
		// If the servers log is empty we can use 0 for last log term and index, because all servers have these two
		// values set to 0 at the start, and they won't refuse the messages with these values
		prevLogTerm = 0
		prevLogIndex = 0
	} else {
		prevLogTerm = server.log[logIndex-1].Term
		prevLogIndex = server.log[logIndex-1].Index
	}
	return prevLogIndex, prevLogTerm
}

// Server log management

func (server *Raft) appendToLog(newLogEntry *log.Message) {
	newLogEntry.Commited = false
	server.log = append(server.log, *newLogEntry)
}

func (server *Raft) commitAllEntriesUpToCommitIndex(commitIndex int) {
	server.writeToFile("Commiting entries\n")
	// startIndex points on log entry that is before the last one, when the appendEntry replication was issued.
	server.commitIndex = commitIndex
	for commitIndex >= 0 && !server.log[commitIndex].Commited {
		server.log[commitIndex].Commited = true
		commitIndex--
	}
	//We increase the index, since it is pointing on entry that was already previously commited
	commitIndex++
	if commitIndex < 0 {
		commitIndex = 0
	}
	for commitIndex < len(server.log) && server.log[commitIndex].Commited {
		server.writeToFile("comitted index " + fmt.Sprintf("%v, ", commitIndex))
		applyMsg := ApplyMsg{
			Index:   commitIndex,
			Command: server.log[commitIndex].Msg,
		}
		server.applyCh <- applyMsg
		commitIndex++
	}
	server.writeToFile("\n")
}

// Writing to log file

func (server *Raft) writeToFile(writeString string) {
	server.writeToFileMutex.Lock()
	defer server.writeToFileMutex.Unlock()
	_, err := server.file.WriteString(writeString)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
}

// Server log replication

func (server *Raft) ClientRequest(in ClientRequestArgs, out *ClientRequestReply) {
	if server.serverState != LEADER {
		fmt.Println("I am not a leader " + fmt.Sprintf("%v", server.me))
		out.Success = false
		out.Position = -1
		return
	}
	server.writeToFile("Received client request\n")

	server.logReplicationMutex.Lock()
	lastLogIndex, lastLogTerm := server.retrieveLastLogIndexAndTerm()
	if lastLogTerm != server.currentTerm {
		lastLogIndex = 0
	}
	newLog := log.Message{
		Term:  server.currentTerm,
		Index: lastLogIndex + 1,
		Msg:   in.Message,
	}
	server.appendToLog(&newLog)
	// Increase the number of logs replicated on this server
	server.nextIndex[server.me]++

	server.resetHeartbeat()
	server.writeToFile("Sending append entry form client request\n")
	fmt.Println("ooooooooooooooouuuuiiiii")
	server.sendAppendEntries()
	fmt.Println("I am the leaderrrrrrrrrrrrrrrrrrrrrr " + fmt.Sprintf("%v", server.me))
	out.Position = len(server.log) - 1
	out.Success = true
	server.logReplicationMutex.Unlock()

	return
}

func (server *Raft) sendAppendEntries() {
	prevLogIndex, prevLogTerm := server.retrievePrevLogIndexAndTerm(len(server.log) - 1)
	logLengthWhenIssuingAppendEntries := len(server.log)
	lastLog := server.log[logLengthWhenIssuingAppendEntries-1]
	appendEntryMessage := AppendEntryArgs{
		Term:         server.currentTerm,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entry: log.Message{
			Term:  lastLog.Term,
			Index: lastLog.Index,
			Msg:   lastLog.Msg,
		},
		LeaderCommit: server.commitIndex,
	}

	var wg sync.WaitGroup
	for index, _ := range server.nextIndex {
		if index == server.me {
			continue
		}
		// If the number of log entries on the current server is for one bigger than the other server,
		// then we can send append entries, otherwise they will be sent with heartbeats
		if server.nextIndex[index] == logLengthWhenIssuingAppendEntries-1 {
			wg.Add(1)
			server.sendAppendEntryMessage(appendEntryMessage, index, logLengthWhenIssuingAppendEntries, &wg)
		}
	}
	// We wait for all the AppendEntry messages to return, only then we allow new AppendEntry to be sent.
	wg.Wait()
}

// Messages sending

func (server *Raft) sendHeartbeatMessage(heartbeatMessage AppendEntryArgs, serverIndex int, waitGroup *sync.WaitGroup) {
	go func() {
		if waitGroup != nil {
			defer waitGroup.Done()
		}
		start := time.Now()
		heartbeatResponse := AppendEntryReply{}
		ok := server.peers[serverIndex].Call("Raft.AppendEntry", heartbeatMessage, &heartbeatResponse, 100)
		elapsed := time.Since(start)
		server.writeToFile("Function took " + fmt.Sprintf("%v\n", elapsed))
		if !ok {
			server.writeToFileMutex.Lock()
			server.writeToFile("Problem with heartbeat\n")
			server.writeToFileMutex.Unlock()
		} else {
			server.writeToFile("Successfully sent heartbeat\n")
			server.writeToFile(fmt.Sprintf("%v\n", server.nextIndex))
			server.heartbeatMutex.Lock()
			server.processHeartbeatResponse(&heartbeatResponse, serverIndex)
			server.heartbeatMutex.Unlock()
		}
	}()
}

func (server *Raft) processHeartbeatResponse(response *AppendEntryReply, serverArrayPosition int) {
	if !response.Success && response.Term > server.currentTerm {
		// We receive success false, because the other server has higher term tha this
		server.becomeCandidate()
		server.changeTerm(response.Term, false)
	} else if !response.Success {
		// We receive success false, because this leader has different log than the follower, to which the appendEntry was sent.
		server.nextIndex[serverArrayPosition]--
	}
}

func (server *Raft) sendAppendEntryMessage(appendEntryMessage AppendEntryArgs, serverIndex int, logLengthToCheckForMajorityReplication int, waitGroup *sync.WaitGroup) {
	go func() {
		if waitGroup != nil {
			defer waitGroup.Done()
		}

		server.writeToFile("starting")
		appendEntryResponse := AppendEntryReply{}
		ok := server.peers[serverIndex].Call("Raft.AppendEntry", appendEntryMessage, &appendEntryResponse, 100)
		server.writeToFile("ending")

		if !ok {
			server.writeToFileMutex.Lock()
			server.writeToFile("Problem with sending Append Entry")
			server.writeToFileMutex.Unlock()
		} else {
			server.processAppendEntryResponseMutex.Lock()
			server.processAppendEntryResponse(&appendEntryResponse, serverIndex, logLengthToCheckForMajorityReplication)
			server.processAppendEntryResponseMutex.Unlock()
		}
	}()
}

func (server *Raft) processAppendEntryResponse(appendEntryResponse *AppendEntryReply, serverIndex int, logLengthToCheckForMajorityReplication int) {
	// Add 1, because the first index is reserved for the current server.
	server.writeToFile("AppendEntryResponse " + fmt.Sprintf("%v\n", appendEntryResponse))
	if appendEntryResponse.Success && server.nextIndex[serverIndex] < server.nextIndex[server.me] {
		server.nextIndex[serverIndex]++
		server.checkIfAppendEntryIsReplicatedOnMajorityOfServers(logLengthToCheckForMajorityReplication)
	} else if !appendEntryResponse.Success && appendEntryResponse.Term > server.currentTerm {
		// We receive success=false, because the other server has higher term
		server.writeToFile(time.Now().String() + " Append entry prvič\n")
		server.becomeCandidate()
		server.changeTerm(appendEntryResponse.Term, false)
	} else if !appendEntryResponse.Success {
		// We receive success=false, because this leader has different log than the follower, to which the appendEntry was sent.
		server.writeToFile(time.Now().String() + " Append entry drugič\n")
		server.nextIndex[serverIndex]--
	}
}

func (server *Raft) checkIfAppendEntryIsReplicatedOnMajorityOfServers(logLengthToCheckForMajorityReplication int) {
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

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	term := rf.currentTerm
	isLeader := rf.serverState == LEADER
	// Your code here.
	return term, isLeader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
}

// example RequestVote RPC handler.
//func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
// Your code here.
//}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	fmt.Println("tttttt")
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply, 100)
	return ok
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
// TODO tole implementiraj
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	if rf.serverState != LEADER {
		return -1, -1, false
	}
	rf.writeToFile("Received request\n")

	heartbeatResponse := ClientRequestReply{}
	rf.peers[rf.me].Call("Raft.ClientRequest", ClientRequestArgs{Message: command}, &heartbeatResponse, 200)
	fmt.Println(heartbeatResponse)
	if heartbeatResponse.Success {
		fmt.Println("success")
		return heartbeatResponse.Position, rf.currentTerm, true
	}
	fmt.Println("fail")

	return -1, rf.currentTerm, true
}

// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {

	rf := CreateServer(peers, me, persister, applyCh)

	// Your initialization code here.

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	return rf
}
