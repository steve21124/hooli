package acceptor

import (
	"fmt"
	"github.com/mburman/hooli/rpc/acceptorrpc"
	"github.com/mburman/hooli/rpc/proposerrpc"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
)

var LOGE = log.New(os.Stderr, "ERROR ", log.Lmicroseconds|log.Lshortfile)
var LOGV = log.New(ioutil.Discard, "VERBOSE ", log.Lmicroseconds|log.Lshortfile)

type acceptorObj struct {
	minProposal     *acceptorrpc.Proposal
	acceptedMessage *proposerrpc.Message
	nextIndex       int
	messages        []proposerrpc.Message
	mutex           *sync.Mutex
	port            int
	filled          []int
}

// port: port to start the acceptorObj on.
func NewAcceptor(port int) *acceptorObj {
	var a acceptorObj
	a.minProposal = &acceptorrpc.Proposal{Number: -1, ID: -1}
	a.acceptedMessage = nil
	a.messages = make([]proposerrpc.Message, 0)
	a.filled = make([]int, 0)
	a.mutex = &sync.Mutex{}
	a.port = port
	a.nextIndex = 0
	setupRPC(&a, port)
	return &a
}

func (a *acceptorObj) Prepare(args *acceptorrpc.PrepareArgs, reply *acceptorrpc.PrepareReply) error {
	fmt.Println("Received Prepare", args)
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if args.Proposal.Number < a.minProposal.Number {
		fmt.Println("CANCEL", a.port)
		reply.Status = acceptorrpc.CANCEL
	} else if args.Proposal.Number == a.minProposal.Number && args.Proposal.ID < a.minProposal.ID {
		fmt.Println("CANCEL EQUAL", a.port)
		reply.Status = acceptorrpc.CANCEL
	} else {
		if a.acceptedMessage != nil {
			// Something was previously accepted.
			fmt.Println("PREV ACCEPTED", a.port)
			reply.Status = acceptorrpc.PREV_ACCEPTED
			reply.AcceptedMessage = *a.acceptedMessage
		} else {
			fmt.Println("OK", a.port)
			reply.Status = acceptorrpc.OK
		}
	}

	a.minProposal = &args.Proposal
	reply.Index = a.nextIndex
	reply.AcceptedProposalNumber = a.minProposal.Number
	return nil
}

func (a *acceptorObj) Accept(args *acceptorrpc.AcceptArgs, reply *acceptorrpc.AcceptReply) error {
	fmt.Println("Received Accept.")
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if args.Proposal.Number < a.minProposal.Number {
		reply.Status = acceptorrpc.CANCEL
	} else if args.Proposal.Number == a.minProposal.Number && args.Proposal.ID < a.minProposal.ID {
		reply.Status = acceptorrpc.CANCEL
	} else {
		// if that position has already been filled
		if args.Index < a.nextIndex && a.filled[args.Index] == 1 {
			reply.Status = acceptorrpc.ALREADY_FILLED
			reply.Message = a.messages[args.Index]
		} else {
			reply.Status = acceptorrpc.OK
			a.acceptedMessage = &args.ProposalMessage
			a.minProposal = &args.Proposal
		}
	}

	reply.MinProposalNumber = a.minProposal.Number
	return nil
}

func (a *acceptorObj) Commit(args *acceptorrpc.CommitArgs, reply *acceptorrpc.CommitReply) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	fmt.Println("Committed message:", args.Message)
	fmt.Println("Index:", args.Index)
	fmt.Println("len messages:", len(a.messages))
	// Increase the size of the array if it is too small.
	if len(a.messages) <= args.Index {
		temp := a.messages
		a.messages = make([]proposerrpc.Message, args.Index+1)
		copy(a.messages, temp)

		// TODO: very inefficient
		temp1 := a.filled
		a.filled = make([]int, args.Index+1)
		copy(a.filled, temp1)
	}
	a.messages[args.Index] = args.Message
	a.filled[args.Index] = 1

	if a.nextIndex == args.Index {
		a.nextIndex = args.Index + 1
		fmt.Println("Incrementing")
	} else if a.nextIndex < args.Index {
		// TODO: missing values... fill
		// just ask for all values

		a.nextIndex = args.Index + 1
		fmt.Println("Incrementing")
	}

	// reset counts
	a.minProposal = &acceptorrpc.Proposal{Number: -1, ID: -1}
	a.acceptedMessage = nil
	return nil
}

func (a *acceptorObj) GetMessages(args *acceptorrpc.GetMessagesArgs, reply *acceptorrpc.GetMessagesReply) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	fmt.Println("Getting ", len(a.messages), " messages")
	reply.Messages = a.messages
	return nil
}

func setupRPC(a *acceptorObj, port int) {
	fmt.Println("Acceptor rpc:", port)
	rpc.RegisterName("AcceptorObj", a)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if e != nil {
		LOGE.Println("listen error:", e)
	}
	go http.Serve(l, nil)
}
