package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/suborbital/grav/discovery/local"
	"github.com/suborbital/grav/grav"
	"github.com/suborbital/grav/transport/websocket"
	"github.com/suborbital/vektor/vk"
	"github.com/suborbital/vektor/vlog"
)

var (
	name     = flag.String("name", "no one", "Name of the instance, for debbuging")
	port     = flag.Int("port", 8080, "Port to start the endpoint on")
	path     = flag.String("path", "/meta/message", "URI path to handle websocket")
	channel  = flag.String("channel", "", "Channel sets the grav message type, no type and uuid are used to check no message is seen agin")
	shutdown = flag.Bool("shutdown", false, "shutdown the mesh")
)

const (
	msgTypeShutdown = "shutdown"
)

func main() {
	flag.Parse()
	logger := vlog.Default(vlog.Level(vlog.LogLevelInfo))
	gwss := websocket.New()
	locald := local.New()
	endChan := make(chan bool, 10)

	var msgType string
	var myUUID uuid.UUID
	if len(*channel) > 0 {
		msgType = *channel
	} else {
		myUUID = uuid.New()
		msgType = myUUID.String()
	}

	g := grav.New(
		grav.UseLogger(logger),
		grav.UseEndpoint(strconv.Itoa(*port), *path),
		grav.UseTransport(gwss),
		grav.UseDiscovery(locald),
		//grav.UseBelongsTo(b2),
		//grav.UseInterests(cap),
	)

	// g.ConnectEndpoint()

	pod := g.ConnectWithReplay()
	defer pod.Disconnect()

	pod.On(func(msg grav.Message) error {
		fmt.Println("received something:", string(msg.Data()))
		if myUUID != uuid.Nil {
			// myUUID is set so check if messages are routed back
			if msg.Type() == myUUID.String() {
				// should not happen we do not want to see our messages again
				panic(msg)
			}
		}
		if msg.Type() == msgTypeShutdown {
			// got a shutdown request, first replicate it to others
			fmt.Println("Replicating shutdown to others")
			pod.ReplyTo(msg, grav.NewMsg(msgTypeShutdown, []byte("shutting down")))
			doShutdown(endChan, pod)
			return nil
		}
		return nil
	})

	// start the websocket
	vk := vk.New(vk.UseAppName("websocket tester"), vk.UseHTTPPort(*port))
	vk.HandleHTTP(http.MethodGet, *path, gwss.HTTPHandlerFunc())
	go vk.Start()

	if *shutdown {
		<-time.After(3 * time.Second)
		fmt.Println("Requesting shutdown of mesh")
		doShutdown(endChan, pod)
		os.Exit(0)
	}

	// generate test messages
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-ticker.C:
			pod.Send(grav.NewMsg(msgType, []byte("hello from "+*name)))
		case <-endChan:
			fmt.Println("Shutdown request")
			return
		}
	}

}

func doShutdown(endChan chan bool, pod *grav.Pod) {

	pod.Send(grav.NewMsg(msgTypeShutdown, []byte("Requesting shutdown"))).WaitUntil(
		grav.Timeout(2),
		func(m grav.Message) error {
			fmt.Println("Got shutdown reply")
			endChan <- true
			return nil
		})
	fmt.Println("Finished shutdown")
}
