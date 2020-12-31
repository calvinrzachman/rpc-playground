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

	// requests is a channel that we send requests for a acceptor response
	// into.
	messages chan learnrpc.MessageInterceptRequest

	// done is closed when the rpc client terminates.
	done chan struct{}

	// quit is a channel for signaling when the RPC server is shutting down
	quit chan struct{}

	wg sync.WaitGroup
}

func newLearnRPCClient() (*learnRPCClient, error) {

	conn, err := grpc.Dial("localhost:8081", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("cannot dial rpc playground server: %v", err)
	}

	rpcClient := learnrpc.NewRPCPlaygroundClient(conn)

	return &learnRPCClient{
		client:   rpcClient,
		messages: make(chan learnrpc.MessageInterceptRequest),
		done:     make(chan struct{}),
	}, nil
}

func main() {
	log.Println("[inside RPC Client]")

	// Build constructor function
	c, _ := newLearnRPCClient()

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := c.client.MessageInterceptor(ctx)
	if err != nil {
		log.Fatal("[inside main()]: unable to connect to the MessageInterceptor RPC ", err)
	}

	c.stream = stream

	// Wait for our goroutines to exit before we return.
	defer func() {
		log.Println("[inside main()]: waiting for go routines to end")
		c.wg.Wait()
	}()

	const workerPoolSize int = 3

	// Channels
	requests := make(chan learnrpc.MessageInterceptRequest)
	responses := make(chan learnrpc.MessageInterceptResponse)
	errorChan := make(chan error)

	log.Println("stream established")

	// Message Initiator
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		message := learnrpc.MessageInterceptRequest{
			ClientID:      "test",
			FirstCriteria: 64,
		}
		for {
			err := c.sendMsgOnEnter(message)
			if err != nil {
				log.Println("[inside message initiator]: error from user input send!")
				return
			}
			select {
			case <-c.done:
				return
			default:
			}
		}
	}()

	// Receive Routine
	c.wg.Add(1)
	go func() {
		c.receiveLoop(responses, errorChan)
		c.wg.Done()
	}()

	// // Worker Pool - Static Deploy
	// // To be used with long-lived workers which continue to proccess multiple messages
	// for j := 0; j < workerPoolSize; j++ {
	// 	c.wg.Add(1)
	// 	go func(j int) {
	// 		c.staticWorker(j, responses, requests, errorChan)
	// 		c.wg.Done()
	// 	}(j)
	// }

	// Send Routine
	c.sendLoop(requests, errorChan)
}

func (c *learnRPCClient) receiveLoop(resp chan learnrpc.MessageInterceptResponse, errChan chan error) {

	for {
		msg, err := c.stream.Recv()
		if err == io.EOF {
			errChan <- err
			return
		}
		if err != nil {
			log.Printf("[inside Receive Routine]: encountered error - %+v", err)
			// close(c.done)
			errChan <- err
			return
		}
		log.Printf("[inside Receive Routine]: %s\n", msg.Error)

		// // UNUSED: Would be used if client processed messages from server and returned them, but for now
		// // we just print any reply from the RPC server.
		// select {
		// // Forward message to worker to begin processing
		// case resp <- *msg:
		// case <-c.done:
		// 	return
		// }
	}
}

func (c *learnRPCClient) sendLoop(req chan learnrpc.MessageInterceptRequest, errChan chan error) error {
	// Close the done channel to indicate that the interceptor is no longer
	// listening and any in-progress requests should be terminated.
	defer func() {
		log.Println("[inside Send Routine]: Closing the 'done' channel")
		close(c.done)
	}()

	for {
		select {
		case initMsg := <-c.messages:
			log.Printf("[inside Send Routine]: Message initiated by external portion of program - %+v Sending to server...\n", initMsg)
			err := c.stream.Send(&initMsg)
			if err != nil {
				log.Println("[inside Send Routine]: error during sen - ", err)
				return err
			}
		// UNUSED: Would be used if client processed messages from server and returned them, but for now
		// we just print any reply from the RPC server.
		case msg := <-req:
			log.Printf("[inside Send Routine]: Received completed request from worker - %+v Sending response...\n", msg)
			err := c.stream.Send(&msg)
			if err != nil {
				return err
			}
		case err := <-errChan:
			log.Println("[inside Send Routine]: error from Receive Routine")
			return err
		}
	}
}

// sendMsgOnEnter blocks and initiates a message upon input from user
func (c *learnRPCClient) sendMsgOnEnter(req learnrpc.MessageInterceptRequest) error {
	log.Println("[inside sendMsgOnEnter]: Press ENTER to send a message")

	var ascii byte
	select {
	case ascii = <-c.asyncUserInput():
	case <-c.done:
		log.Println("[inside sendMsgOnEnter]: Done channel closed!")
		return nil
	}

	// ESC = 27 and Ctrl-C = 3
	if ascii == 27 || ascii == 3 {
		fmt.Println("Exiting...")
		return nil
	}

	c.messages <- req

	return nil
}

// asyncUserInput spins a go routine which waits for a single byte of user input
// and writes any input received to a channel. We return the channel immediately
// so that it may be used in the select statement to prevent hanging waiting for
// user input in the event our RPC routines end.
func (c *learnRPCClient) asyncUserInput() chan byte {
	asyncChan := make(chan byte)

	go func() {
		// only read single characters, the rest will be ignored!!
		// log.Println("[inside asyncUserInput]: Waiting for user input...")
		consoleReader := bufio.NewReaderSize(os.Stdin, 1)
		fmt.Print(">")
		input, _ := consoleReader.ReadByte()
		ascii := input
		// log.Println("[inside asyncUserInput]: Received user input!")
		select {
		case asyncChan <- ascii:
		case <-c.done:
			return
		}
	}()

	return asyncChan
}
