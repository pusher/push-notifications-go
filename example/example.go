package main

import (
	"fmt"

	"github.com/pusher/push-notifications-go"
)

const (
	instanceId = "8a070eaa-033f-46d6-bb90-f4c15acc47e1"
	secretKey  = "A84891378A575A220BEB53352E84385"
)

func main() {
	notifications :=
		pushnotifications.New(instanceId, secretKey)

	publishRequest := map[string]interface{}{
		"apns": map[string]interface{}{
			"aps": map[string]interface{}{
				"alert":  map[string]interface{}{
					"title": "Hello",
					"body": "Hello, world",
				},
			},
		},
		"fcm": map[string]interface{}{
			"notification": map[string]interface{}{
				"title": "Hello",
				"body":  "Hello, world",
			},
		},
	}
	pubID, err := notifications.Publish([]string{"hello"}, publishRequest)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Publish ID:", pubID)
	}
}
