package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/calvinrzachman/rpc-playground/learnrpc"
	"google.golang.org/grpc"
)

type learnRPCClient struct {
	client learnrpc.RPCPlaygroundClient
	stream learnrpc.RPCPlayground_MessageInterceptorClient

	// done is closed when the rpc client terminates.
	done chan struct{}

	// quit is a channel for signaling when the RPC server is shutting down
	quit chan struct{}

	wg sync.WaitGroup
}

func main() {
	log.Println("[inside RPC Client]")

	opts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithBlock(),
	}

	conn, err := grpc.Dial("localhost:8081", opts...)
	if err != nil {
		log.Fatalf("cannot dial rpc playground server: %v", err)
	}

	rpcClient := learnrpc.NewRPCPlaygroundClient(conn)

	stream, err := rpcClient.MessageInterceptor(context.Background())
	if err != nil {
		log.Fatal("[inside RPC Client]: unable to connect to the MessageInterceptor RPC ", err)
	}

	client := learnRPCClient{
		client: rpcClient,
		stream: stream,
	}

	// Wait for our goroutines to exit before we return.
	defer func() {
		log.Println("[inside RPC Client main() method]: waiting for go routines to end")
		client.wg.Wait()
	}()

	const workerPoolSize int = 3

	// Channels
	requests := make(chan learnrpc.MessageInterceptRequest)
	responses := make(chan learnrpc.MessageInterceptResponse)
	errorChan := make(chan error)

	log.Println("stream established")
	client.wg.Add(1)
	go func() {
		client.receiveLoop(responses, errorChan)
		client.wg.Done()
	}()

	// // Worker Pool - Static Deploy
	// // To be used with long-lived workers which continue to proccess multiple messages
	// for j := 0; j < workerPoolSize; j++ {
	// 	client.wg.Add(1)
	// 	go func(j int) {
	// 		client.staticWorker(j, responses, requests, errorChan)
	// 		client.wg.Done()
	// 	}(j)
	// }

	client.sendLoop(requests, errorChan)
}

func (c *learnRPCClient) receiveLoop(req chan learnrpc.MessageInterceptResponse, errChan chan error) {

	for {
		msg, err := c.stream.Recv()
		if err == io.EOF {
			errChan <- err
			return
		}
		if err != nil {
			log.Printf("[inside Receive Routine]: %+v", err)
			errChan <- err
			return
		}
		log.Printf("[inside Receive Routine]: %s\n", msg.Error)

		// select {
		// // Forward message to worker to begin processing
		// case req <- *msg:
		// case <-c.done:
		// 	return
		// 	// case errChan <- err:
		// }
	}
}

func (c *learnRPCClient) sendLoop(resp chan learnrpc.MessageInterceptRequest, errChan chan error) error {
	// Close the done channel to indicate that the interceptor is no longer
	// listening and any in-progress requests should be terminated.
	defer func() {
		log.Println("[inside Send Routine]: Closing the 'done' channel")
		// close(c.done)
	}()

	for {
		select {
		case msg := <-resp:
			log.Printf("[inside Send Routine]: Received completed request from worker - %+v Sending response...\n", msg)
			err := c.stream.Send(&msg)
			if err != nil {
				return err
			}
		case err := <-errChan:
			log.Println("[inside Send Routine]: error from Receive Routine")
			return err
		default:
			message := learnrpc.MessageInterceptRequest{
				ClientID:      "test",
				FirstCriteria: 64,
			}
			c.sendMsgOnEnter(message)
		}
	}
}

// staticWorker is a long lived worker that processes work continuously in a loop
func (c *learnRPCClient) staticWorker(j int, resp chan learnrpc.MessageInterceptResponse, req chan learnrpc.MessageInterceptRequest, errChan chan error) {

	var workItem learnrpc.MessageInterceptRequest

	// for request := range req {
	for {
		select {
		case request := <-resp:
			// Do work ...
			log.Printf("[inside Worker %d]: Received Work Item: %+v\n", j, request)

			// Transform workItem into expected reply format
			workItem = learnrpc.MessageInterceptRequest{
				ClientID: "test",
			}

			// Trigger Send Loop
			req <- workItem
		case <-c.done:
			return
		}
	}
}

func (c *learnRPCClient) sendMsgOnEnter(req learnrpc.MessageInterceptRequest) error {
	log.Println("[inside sendMsgOnEnter]: Press ENTER to send a message")

	// only read single characters, the rest will be ignored!!
	consoleReader := bufio.NewReaderSize(os.Stdin, 1)
	fmt.Print(">")
	input, _ := consoleReader.ReadByte()
	ascii := input

	// ESC = 27 and Ctrl-C = 3
	if ascii == 27 || ascii == 3 {
		fmt.Println("Exiting...")
	}

	select {
	case <-c.done:
		return nil
	default:
	}

	err := c.stream.Send(&req)
	if err != nil {
		log.Println("[inside sendMsgOnEnter]: unable to send message ", err)
		return err
	}

	return nil
}

func async() chan bool {
	return nil
}
