package main

import (
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"context"
	"encoding/json"
	fluxapi "github.com/fluxcd/flux/pkg/api"
	v9 "github.com/fluxcd/flux/pkg/api/v9"
	fluxhttp "github.com/fluxcd/flux/pkg/http"
	fluxclient "github.com/fluxcd/flux/pkg/http/client"
	fluxdaemon "github.com/fluxcd/flux/pkg/http/daemon"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"log"
	"net/http"
	"testing"
	"time"
)

// example message from: https://cloud.google.com/source-repositories/docs/pubsub-notifications#notification_example
const testMsg = `{
  "name": "projects/test-project/repos/pubsub-test",
  "url": "[URL_PATH]",
  "eventTime": "2018-02-21T21:23:25.566175Z",
  "refUpdateEvent": {
    "email": "someone@somecompany.com",
    "refUpdates": {
      "refs/heads/master": {
      "refName": "refs/heads/master",
      "updateType": "UPDATE_FAST_FORWARD",
      "oldId": "c7a28dd5de3403cc384a025834c9fce2886fe763",
      "newId": "f00768887da8de62061210295914a0a8a2a38226"
      }
    }
  }
}`

var notification sourceRepoNotification

func init() {
	if err := json.Unmarshal([]byte(testMsg), &notification); err != nil {
		log.Fatalf("Couldn't unmarshal pubsub message data, %v", err)
	}
}

func TestUnmarshal(t *testing.T) {
	var notification sourceRepoNotification
	err := json.Unmarshal([]byte(testMsg), &notification)

	if err != nil {
		t.Error("couldn't unmarshal test message")
	}
}

func TestConsume(t *testing.T) {
	ctx := context.Background()
	// Start a fake server running locally.
	srv := pstest.NewServer()
	defer srv.Close()
	// Connect to the server without using TLS.
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Use the connection when creating a pubsub client.
	client, err := pubsub.NewClient(ctx, "project", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	log.Print("creating topic")

	topic, err := client.CreateTopic(ctx, "projects/project/topics/testTopic")
	if err != nil {
		t.Fatal(err)
	}

	log.Print("topic created")

	cm := make(chan *pubsub.Message)

	consumeCtx, cancel := context.WithCancel(ctx)
	go func() {
		log.Print("awaiting messages...")
		for msg := range cm {
			log.Printf("got msg, %v", msg.ID)
			cancel()
		}
	}()

	sub, err := prepare(consumeCtx, client, cm, "projects/project/topics/testTopic", "testSub", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	id, err := topic.Publish(ctx, &pubsub.Message{
		Data: []byte(testMsg),
	}).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("published msg, %v", id)

	err = consume(consumeCtx, sub, cm)
	if err != nil {
		log.Fatal(err)
	}
}

func TestHandle(t *testing.T) {
	handler := fluxdaemon.NewHandler(MockServer{
		T: t,
	}, fluxdaemon.NewRouter())
	httpServer := http.Server{
		Addr:    "127.0.0.1:3030",
		Handler: handler,
	}
	defer httpServer.Close()

	go func() {
		err := httpServer.ListenAndServe()
		if err != nil {
			t.Fatal(err)
		}
	}()

	apiClient := fluxclient.New(http.DefaultClient, fluxhttp.NewAPIRouter(), "http://localhost:3030", fluxclient.Token(""))

	err := handleMsg(context.Background(), notification, apiClient, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

type MockServer struct {
	fluxapi.Server

	T *testing.T
}

func (ms MockServer) NotifyChange(ctx context.Context, change v9.Change) error {
	switch change.Source.(type) {
	case v9.GitUpdate:
		assert.Equal(ms.T, "master", change.Source.(v9.GitUpdate).Branch)
	default:
		ms.T.Fatal("change source type didn't match")
	}
	return nil
}
