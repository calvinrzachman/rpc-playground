# rpc-playground

The RPC Playground is intended for use in learning how to use gRPC. Primary focus is given to understanding the structure of bi-directional streams between client and server.

Paradigm:
- RPC Driver/Main Method
  - Send Routine
  - Receive Routine
  - Worker Routine(s)
  
The basic structure is as follows:

```golang
// Run is a driver function for a bi-directional RPC
// of the "simple responder" type. Requests are received by
// a dedicated Go routine, piped to a set of Worker routines
// for message processing/validation, and then passed to a dedicated
// Go routine for returning a response to gRPC stream.
func (r *Responder) Run() {

	// Channels
	requests := make(chan struct{})
	responses := make(chan struct{})
	errorChan := make(chan error)

	// Pure Receive Loop/Routine
	go receiveLoop(requests, errorChan)

	// Worker Pool - Static Deploy
	// To be used with long-lived workers which continue to proccess multiple messages
	for i := range workerPoolSize {
		go staticWorker(requests, response, errorChan)
	}

	// Pure Send Loop/Routine
	sendLoop(responses, errorChan)
  
}

// Run is a driver function for a bi-directional RPC
// of the "message initiator" type. Requests are initiated from some part 
// of the Go program and passed via a pre-established channel to a dedicated
// Go routine which forwards them to the remote side of the gRPC connection.
func (r *Initiator) Run() {

	// Channels
	errorChan := make(chan error)

	// Pure Receive Loop/Routine
	go receiveLoop(errorChan)

	// Pure Send Loop/Routine
        // This routine sends messages as it receives them from the r.requests channel.
	// The r.requests channel has previously been shared with some other (likely stateful)
	// portion of our program (the message initiator). 
	sendLoop(r.requests, errorChan)
  
}
```
  
How will we know when it is time to send a message?
		We are either:
		- Responding to a received message - in which case the Receive routine will notify us (common for clients)
		- Initiating a message of our own - in which case we will be notified by some other component of our software (common for servers)
  
Both the client and server are able to continously send messages using a bi-directional stream. In practice, there does tend to be a *message initiator* and a *message responder*. Such is the case with `lnd`'s ***ChannelAcceptor*** and ***HTLCInterceptor*** RPCs.

Persistent streaming RPCs allow us to amortize the TLS/TCP connection negotiation cost across multiple RPC calls and even multiple RPC streams.
