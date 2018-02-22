package main

import (
	"fmt"

	"github.com/pusher/push-notifications-go"
)

const (
	instanceId = "96f84bc1-075f-4ac6-8a1b-eba0a54886f7"
	secretKey  = "3B397552E080252048FE03009C1253A"
)

func main() {
	notifications, _ := pushnotifications.New(instanceId, secretKey)

	publishRequest := map[string]interface{}{
		"apns": map[string]interface{}{
			"aps": map[string]interface{}{
				"alert": map[string]interface{}{
					"title": "Hello",
					"body":  "Hello, world",
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

	pubId, err := notifications.Publish([]string{"hello"}, publishRequest)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Publish Id:", pubId)
	}
}
