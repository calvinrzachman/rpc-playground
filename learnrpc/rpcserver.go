package learnrpc

import (
	fmt "fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
)

// Server is a gRPC server
type Server struct {
	// started sync.Once
	// stopped sync.Once

	cfg *Config

	wg sync.WaitGroup

	// quit is a channel for signaling when the RPC server is shutting down
	quit chan struct{}
}

// Config defines the configuration options for the RPC Playground server.
type Config struct{}

// NewServer ...
func NewServer(quit chan struct{}) *Server {
	return &Server{
		cfg:  &Config{},
		quit: quit,
	}
}

// interceptor carries the functionality necessary to implement the MessageInterceptor RPC
type interceptor struct {
	// stream is the bi-directional MessageInterceptor gRPC stream
	stream RPCPlayground_MessageInterceptorServer

	// done is closed when the rpc client terminates.
	done chan struct{}

	// quit is a channel for signaling when the RPC server is shutting down
	quit chan struct{}

	wg sync.WaitGroup
}

// newInterceptor creates and returns an instance of the interceptor.
func newInterceptor(stream RPCPlayground_MessageInterceptorServer) *interceptor {
	return &interceptor{
		stream: stream,
		done:   make(chan struct{}),
	}
}

// Driver (Send/Receive loop construction) method for MessageInterceptor RPC
func (i *interceptor) run() error {
	// Wait for our goroutines to exit before we return.
	defer func() {
		log.Println("[inside Interceptor run()]: waiting for go routines to end")
		i.wg.Wait()
	}()

	const workerPoolSize int = 3

	// Channels
	requests := make(chan MessageInterceptRequest)
	responses := make(chan MessageInterceptResponse)
	errorChan := make(chan error)

	i.wg.Add(1)
	// Pure Receive Loop
	go func() {
		i.receiveLoop(requests, errorChan)
		i.wg.Done()
	}()

	// Worker Pool - Static Deploy
	// To be used with long-lived workers which continue to proccess multiple messages
	for j := 0; j < workerPoolSize; j++ {
		i.wg.Add(1)
		go func(j int) {
			i.staticWorker(j, requests, responses, errorChan)
			i.wg.Done()
		}(j)
	}

	// Pure Send Loop
	return i.sendLoop(responses, errorChan)
}

func (i *interceptor) receiveLoop(req chan MessageInterceptRequest, errChan chan error) {

	ctx := i.stream.Context()

	for {
		msg, err := i.stream.Recv()
		if err != nil {
			log.Printf("[inside Receive Routine]: error on receive %+v", err)
			errChan <- err
			return
		}
		log.Println("[inside Receive Routine]: Received message from client! Sending to worker!")

		// Optionally forward to worker inside of own go routine
		// Running Send/Receive functions inside their own routine allows for us to send and receive messages
		// at the same time. We still will block here attempting to send a message to one of our workers.
		// We will be unable to receive any more messages until one of the workers frees up to read from the channel.
		// NOTE: To observe this, just up the message process time. Effectively the worker count limits the concurrent
		// message processing. We could alternatively manage a message queue from which the workers can draw. That way
		// we can build backlog of work for our workers to get to when they can. There does not seem much point in spawning
		// go routines just to sit idly by waiting for an open worker rather than. Either we maintain a buffer for message overflow
		// or we rely on gRPC/transport.
		// NOTE: We should always implement some method to control the level of concurrency/# of Go routines
		// Interestingly gRPC ensures that all messages get delivered regardless of whether we do this!
		// go func() {
		select {
		// Forward message to worker to begin processing
		case req <- *msg:
		case <-ctx.Done():
			log.Println("[inside Receive Routine]: Context Done!")
			return
		case <-i.done:
			log.Println("[inside Receive Routine]: Shutting Down!")
			return
		}
		// }()
	}
}

func (i *interceptor) sendLoop(resp chan MessageInterceptResponse, errChan chan error) error {
	// Close the done channel to indicate that the interceptor is no longer
	// listening and any in-progress requests should be terminated.
	defer func() {
		log.Println("[inside Send Routine]: Closing the 'done' channel")
		close(i.done)
	}()

	ctx := i.stream.Context()

	for {
		select {
		// Send messages back to client as worker routines complete processing
		case msg := <-resp:
			log.Printf("[inside Send Routine]: Sending completed request from worker - %+v\n", msg)
			err := i.stream.Send(&msg)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			log.Println("[inside Send Routine]: Context Done!")
			return nil
		case err := <-errChan:
			return err
		}
	}
}

// staticWorker is a long lived worker that processes work continuously in a loop
func (i *interceptor) staticWorker(j int, req chan MessageInterceptRequest, resp chan MessageInterceptResponse, errChan chan error) {

	var workItem MessageInterceptResponse

	for {
		select {
		case request := <-req:
			// Do work ...

			// Transform workItem into expected reply format
			workItem = MessageInterceptResponse{
				Accept: true,
				Error:  fmt.Sprintf("This is a reply from server Go routine %d with criteria - %d", j, request.FirstCriteria),
			}

			// Simulate message processing
			log.Printf("[inside Worker %d]: Processing...\n", j)

			// Worker #2 is speedy
			// Watch as worker 2 is able to accept and handle many requests in the time
			// workers 0 and 1 are still processing
			var n int
			if j == 2 {
				n = 1
			} else {
				// Other workers are not
				n = 10
			}

			// Other workers are typically not
			// rand.Seed(time.Now().UnixNano())
			// n = rand.Intn(10)
			time.Sleep(time.Duration(n) * time.Second)
			// time.Sleep(5 * time.Second)
			log.Printf("[inside Worker %d]: Done!\n", j)

			// Trigger Send Routine
			resp <- workItem

		case <-i.done:
			log.Printf("[inside Worker %d]: Shutting down worker!\n", j)
			return
		}

	}
}

// MessageInterceptor ...
func (s *Server) MessageInterceptor(stream RPCPlayground_MessageInterceptorServer) error {
	log.Println("[inside MessageInterceptor RPC]: Starting up the interceptor!")

	// Either implement Send/Receive Loops right here OR create a driver type and call its main/run method
	interceptor := newInterceptor(stream)

	// Optional Plugin Point: This would also likely be the location where we can extend message
	// initiation ability to another part of our program
	// something.Add(interceptor)

	// Run the message interceptor
	return interceptor.run()

}

// Start the RPCPlayground gRPC server
func (s *Server) Start() error {

	// Setup gRPC Payment Request streaming server
	srv := grpc.NewServer()

	RegisterRPCPlaygroundServer(srv, s)

	l, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatalf("[main routine]: could not listen on :8081: %v", err)
		return err
	} else {
		log.Println("[main routine]: listening on :8081")
	}

	if err := srv.Serve(l); err != nil {
		log.Fatalf("[main routine]: failed to serve: %s", err)
		return err
	}

	return nil
}

// A compile time check to ensure that Server fully implements the
// RPCPlaygroundServer gRPC service.
var _ RPCPlaygroundServer = (*Server)(nil)

/*

	Bi-directional RPC Structure/Ideas


	func receiveLoop(req chan struct{}, errChan chan error) {
		for {
			msg, err := stream.Recv()

			select {
			// Forward message to worker to begin processing
			case req <- msg:
			case errChan <- err:
			}
		}
	}

	// NOTE: Should the workers be long lived/capable of processing many requests?
	// If started with for loop from driver function -> Yes (otherwise new workers won't get created)
	//

	func sendLoop(resp chan struct{}, errChan chan error) {
		for {
			select {
			case msg := <-resp:
				stream.Send(msg)
			}
		}
	}

	// Run is a driver function
	func Run() {

		// Channels
		requests := make(chan struct{})
		responses := make(chan struct{})
		errorChan := make(chan error)

		// Pure Receive Loop
		go receiveLoop(requests, errorChan)

		// Worker Pool - Static Deploy
		// To be used with long-lived workers which continue to proccess multiple messages
		for i := range workerPoolSize {
			go staticWorker(requests, response, errorChan)
		}

		// Pure Send Loop
		sendLoop(responses, errorChan)

		// Worker Pool - Continuous Deploy
		// To be used with short lived workers which die after message processing
		// Could also be used to make worker uptime more robust by restarting workers
		// should any of them quit working
		for finished := range workerLifecycleChan {
			go continuousWorker(requests, response, errorChan)
		}
	}


	// staticWorker is a long lived worker that processes work continuously in a loop
	func staticWorker(req, resp chan struct{}, errChan chan error) {
		for request := range req {
			// Do work ...

			// Transform workItem into expected reply format

			// Trigger Send Loop
			resp <- workItem
		}
	}

	// Paradoxically the continous worker does not work continously, but rather
	// does some work then dies. When used as part of a Continuous worker deployment
	// he will be spun back up. This seems to increase Go routine CHURN so unless
	// I can come up with a good rationale for using it, I should probably use staticWorker
	func continuousWorker(req, resp chan struct{}, errChan chan error) {
		select {
		case workItem := <-req:
			// Do work ...

			// Transform workItem into expected reply format

			// Trigger Send Loop
			resp <- workItem

		}
	}

*/
