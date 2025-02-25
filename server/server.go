package server

import (
	"context"
	"math/rand"
	"time"
)

const LEADER = 1
const FOLLOWER = 2
const CANDIDATE = 3

type Server struct {
	currentTerm int
	votedFor    int
	log         []string
	serverState int
	timer       time.Duration

	//volatile state on every server
	commitIndex int
	//TODO tale lastApplied ni nujno potreben
	lastApplied int

	//volatile state on leader
	nextIndex []int
	//TODO tole tudi nevem zakaj točno se rabi
	matchIndex []int
}

func begin() {
	server := Server{}
	server.currentTerm = 0
	server.votedFor = 0
	server.serverState = FOLLOWER
	server.commitIndex = 0
	server.resetTimeout()
}

func (server Server) resetTimeout() {
	server.timer = time.Millisecond * time.Duration(rand.Intn(150+rand.Intn(150)))
}

func (server Server) recievedValidRPC(newLog string) {
	server.resetTimeout()
	server.log[server.lastApplied] = newLog
	//TODO
}

func (server Server) electionTimeout() {
	server.becomeCandidate()
	beginElection()
}

func (server Server) becomeCandidate() {
	server.serverState = CANDIDATE
}

func beginElection() {

}

func recieveRPCWithStaleTerm() {

}

func recieveValidHeartbeat() {

}

func voteRequest() {

}

func (s *Server) AppendEntry(ctx context.Context, in *pb.AppendEntryMessage) (*pb.AppendEntryResponse, error) {
	return &pb.HelloReply{Message: "Hello again " + in.GetName()}, nil
}
