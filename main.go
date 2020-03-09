package main

import (
	"cloud.google.com/go/pubsub"
	"context"
	"encoding/json"
	fluxapi_v9 "github.com/fluxcd/flux/pkg/api/v9"
	fluxhttp "github.com/fluxcd/flux/pkg/http"
	fluxclient "github.com/fluxcd/flux/pkg/http/client"
	"github.com/prometheus/common/log"
	flag "github.com/spf13/pflag"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultApiBase = "http://localhost:3030/api/flux"

type updateType int

const (
	create updateType = iota
	updateFastForward
	updateNonFastForward
	delete
)

var updateTypes = map[string]updateType{
	"CREATE":                  create,
	"UPDATE_FAST_FORWARD":     updateFastForward,
	"UPDATE_NON_FAST_FORWARD": updateNonFastForward,
	"DELETE":                  delete,
}

type sourceRepoRefUpdate struct {
	RefName    string `json:"refName"`
	UpdateType string `json:"updateType"`
	OldId      string `json:"oldId"`
	NewId      string `json:"newId"`
}

type sourceRepoRefUpdateEvent struct {
	Email      string                         `json:"email"`
	RefUpdates map[string]sourceRepoRefUpdate `json:"refUpdates"`
}

type sourceRepoNotification struct {
	Name           string                   `json:"name"`
	Url            string                   `json:"url"`
	EventTime      string                   `json:"eventTime"`
	RefUpdateEvent sourceRepoRefUpdateEvent `json:"refUpdateEvent"`
}

func main() {
	var (
		projectId   string
		subId       string
		topicId     string
		syncTimeout time.Duration
	)

	flags := flag.NewFlagSet("flux-recv-gcsr", flag.ExitOnError)

	flags.StringVar(&projectId, "projectId", "", "project id where pubsub topic is located")
	flags.StringVar(&subId, "subId", "", "subscription id to consume messages from")
	flags.StringVar(&topicId, "topicId", "", "topic id to subscribe to")
	flags.DurationVar(&syncTimeout, "syncTimeout", 30*time.Second, "flux sync timeout")

	flags.Parse(os.Args[1:])

	ctx := context.Background()

	if subId == "" {
		log.Fatalf("No subscription id provided")
	}

	if projectId == "" {
		credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeReadonlyScope)
		if err != nil {
			log.Fatal(err)
		}
		projectId = credentials.ProjectID
	}

	client, err := pubsub.NewClient(ctx, projectId)
	if err != nil {
		log.Fatalf("pubsub.NewClient: %v", err)
	}
	defer client.Close()

	cm := make(chan *pubsub.Message)

	apiClient := fluxclient.New(http.DefaultClient, fluxhttp.NewAPIRouter(), defaultApiBase, fluxclient.Token(""))

	go func() {
		handleLoop(ctx, cm, apiClient, syncTimeout)
	}()

	sub, err := prepare(ctx, client, cm, topicId, subId, syncTimeout)
	if err != nil {
		log.Fatal(err)
	}

	err = consume(ctx, sub, cm)
	if err != nil {
		log.Fatal(err)
	}
}

func prepare(ctx context.Context, client *pubsub.Client, cm chan *pubsub.Message, topicId string, subId string, syncTimeout time.Duration) (*pubsub.Subscription, error) {
	var err error
	var sub *pubsub.Subscription

	log.Info("Preparing consumer loop")

	if topicId != "" {
		log.Infof("Attempting to create subscription, %v, for topic, %v", subId, topicId)

		sub, err = client.CreateSubscription(ctx, subId, pubsub.SubscriptionConfig{
			Topic:       client.Topic(topicId),
			AckDeadline: syncTimeout,
			Labels:      nil,
		})
		if err != nil {
			switch status.Code(err) {
			case codes.AlreadyExists:
				log.Infof("Subscription already exists: %v", err)
			case codes.NotFound:
				log.Errorf("Topic not found: %v", err)
				return nil, err
			}
		}
	}

	if sub == nil {
		sub = client.Subscription(subId)
	}

	log.Infof("Got subscription, %v", subId)

	// Turn on synchronous mode. This makes the subscriber use the Pull RPC rather
	// than the StreamingPull RPC, which is useful for guaranteeing MaxOutstandingMessages,
	// the max number of messages the client will hold in memory at a time.
	sub.ReceiveSettings.Synchronous = true
	sub.ReceiveSettings.MaxOutstandingMessages = 10

	return sub, nil
}

func handleLoop(ctx context.Context, cm chan *pubsub.Message, apiClient *fluxclient.Client, syncTimeout time.Duration) {
	for {
		select {
		case msg := <-cm:
			var notification sourceRepoNotification
			if err := json.Unmarshal(msg.Data, &notification); err != nil {
				log.Fatalf("Couldn't unmarshal pubsub message data, %v", err)
			}

			err := handleMsg(ctx, notification, apiClient, syncTimeout)
			if err == nil {
				msg.Ack()
			} else {
				msg.Nack()
			}
		case <-ctx.Done():
			close(cm)
			return
		}
	}
}

func handleMsg(ctx context.Context, notification sourceRepoNotification, apiClient *fluxclient.Client, syncTimeout time.Duration) error {
	for _, ref := range notification.RefUpdateEvent.RefUpdates {
		update := fluxapi_v9.GitUpdate{
			URL:    notification.Url,
			Branch: strings.TrimPrefix(ref.RefName, "refs/heads/"),
		}
		change := fluxapi_v9.Change{
			Kind:   fluxapi_v9.GitChange,
			Source: update,
		}

		err := func() error {
			ctx, cancel := context.WithTimeout(ctx, syncTimeout)
			defer cancel()
			err := apiClient.NotifyChange(ctx, change)
			if err != nil {
				select {
				case <-ctx.Done():
					log.Warnf("Timed out waiting for response from downstream API: %v", err)
				default:
					log.Errorf("Error while calling downstream API: %v", err)
				}
				return err
			}
			return nil
		}()

		if err != nil {
			return err
		}
	}

	return nil
}

func consume(ctx context.Context, sub *pubsub.Subscription, cm chan *pubsub.Message) error {
	// Receive blocks until the passed in context is done.
	err := sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		log.Infof("Message received, %v", msg.ID)
		cm <- msg
	})
	if err != nil && status.Code(err) != codes.Canceled {
		return err
	}
	return nil
}
