package main

import (
	"encoding/json"
	"net/http"

	"github.com/pusher/push-notifications-go"
)

const (
	instanceId = "96f84bc1-075f-4ac6-8a1b-eba0a54886f7"
	secretKey  = "3B397552E080252048FE03009C1253A"
)

func main2() {
	beamsClient, _ := pushnotifications.New(instanceId, secretKey)

	http.HandleFunc("/pusher/beams-auth", func (w http.ResponseWriter, r *http.Request) {
		// Do your normal auth checks here 🔒
		userID := "" // get it from your auth system
		userIDinQueryParam := r.URL.Query().Get("user_id")
		if userID != userIDinQueryParam {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		beamsToken, err := beamsClient.GenerateToken(userID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		beamsTokenJson, err := json.Marshal(beamsToken)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(beamsTokenJson)
	})

	http.ListenAndServe(":8080", nil)
}
