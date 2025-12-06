package main

import (
	"fmt"
	"math/rand"
	sgrpc "raftImplementation/raft/server/src"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func main() {
	start := time.Now()
	wg := sync.WaitGroup{}
	var lock sync.Mutex
	var maxDur time.Duration
	var maxInt int
	for numOfGoRutines := 0; numOfGoRutines < 200; numOfGoRutines++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := grpc.NewClient("192.168.2.190:5000", grpc.WithInsecure())
			if err != nil {
				panic(err)
			}
			defer conn.Close()
			grpcClient := sgrpc.NewServerServiceClient(conn)
			contextServer, cancel := context.WithTimeout(context.Background(), time.Second*25)
			defer cancel()
			for numOfClientRequests := 0; numOfClientRequests < 1250; numOfClientRequests++ {
				maxDur, maxInt = sendClientRequest(numOfGoRutines, grpcClient, contextServer, &lock, maxDur, maxInt)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("Time elapsed: %s\n", elapsed)
	fmt.Println(maxDur)
	fmt.Println(maxInt)
}

func sendClientRequest(i int, grpcClient sgrpc.ServerServiceClient, contextServer context.Context, lock *sync.Mutex, maxDur time.Duration, maxInt int) (time.Duration, int) {
	start2 := time.Now()
	appendEntryMessage := sgrpc.ClientRequestMessage{
		Message: strconv.Itoa(i),
	}
	response, err := grpcClient.ClientRequest(contextServer, &appendEntryMessage)
	elapsed2 := time.Since(start2)
	lock.Lock()
	if elapsed2 > maxDur {
		maxDur = elapsed2
		maxInt = i
		fmt.Println(i, " time ", elapsed2)
	}
	lock.Unlock()
	if err != nil {
		panic(err)
	}
	if i == 0 {
		fmt.Println(response)
	}
	return maxDur, maxInt
}

func stringWithCharset(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
