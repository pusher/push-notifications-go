package pushnotifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/pkg/errors"
)

type PushNotifications interface {
	// Publishes notifications to all devices subscribed to at least 1 of the interests given
	// Returns a non-empty `publishId` string if successful; or a non-nil `error` otherwise.
	Publish(interests []string, request map[string]interface{}) (publishId string, err error)
}

const (
	defaultRequestTimeout     = time.Minute
	defaultBaseEndpointFormat = "https://%s.pushnotifications.pusher.com"
)

var (
	NoInterestsSuppliedErr           = errors.New("No interests were supplied")
	TooManyInterestsSuppliedErr      = errors.New("Too many interests supplied: API only supports up to 10.")
	InterestNameTooShortErr          = errors.New("An empty interest name is not valid")
	InterestNameTooLongErr           = errors.New("Interest length is over 164 characters")
	InterestWithInvalidCharactersErr = errors.New(
		"An interest name can be at most 164 characters long and contain: ASCII upper/lower-case letters, numbers and one of _=@,.:")

	interestValidationRegex = regexp.MustCompile("^[a-zA-Z0-9_=@,.;]+$")
)

type pushNotifications struct {
	InstanceId string
	SecretKey  string

	baseEndpoint string
	httpClient   *http.Client
}

func New(instanceId string, secretKey string) PushNotifications {
	return &pushNotifications{
		InstanceId: instanceId,
		SecretKey:  secretKey,

		baseEndpoint: fmt.Sprintf(defaultBaseEndpointFormat, instanceId),
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}
}

type publishResponse struct {
	PublishId string `json:"publishId"`
}

type publishErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"description"`
}

func (pn *pushNotifications) Publish(interests []string, request map[string]interface{}) (string, error) {
	if len(interests) == 0 {
		// this request was not very interesting :/
		return "", NoInterestsSuppliedErr
	}

	if len(interests) > 10 {
		return "", TooManyInterestsSuppliedErr
	}

	for _, interest := range interests {
		if len(interest) == 0 {
			return "", InterestNameTooShortErr
		}

		if len(interest) > 164 {
			return "", InterestNameTooLongErr
		}

		if !interestValidationRegex.MatchString(interest) {
			return "", InterestWithInvalidCharactersErr
		}
	}
	request["interests"] = interests

	bodyRequestBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "Failed to marshal the publish request JSON body")
	}

	url := fmt.Sprintf(pn.baseEndpoint+"/publish_api/v1/instances/%s/publishes", pn.InstanceId)

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyRequestBytes))
	if err != nil {
		return "", errors.Wrap(err, "Failed to prepare the publish request")
	}

	httpReq.Header.Add("Authorization", "Bearer "+pn.SecretKey)
	httpReq.Header.Add("Content-Type", "application/json")
	httpReq.Header.Add("X-Pusher-Library", "pusher-push-notifications-go "+sdkVersion)

	httpResp, err := pn.httpClient.Do(httpReq)
	if err != nil {
		return "", errors.Wrap(err, "Failed to publish notifications due to a network error")
	}

	defer httpResp.Body.Close()
	responseBytes, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return "", errors.Wrap(err, "Failed to read publish notification response due to a network error")
	}

	switch httpResp.StatusCode {
	case http.StatusOK:
		pubResponse := &publishResponse{}
		err = json.Unmarshal(responseBytes, pubResponse)
		if err != nil {
			return "", errors.Wrap(err, "Failed to read publish notification response due to invalid JSON")
		}

		return pubResponse.PublishId, nil
	default:
		pubErrorResponse := &publishErrorResponse{}
		err = json.Unmarshal(responseBytes, pubErrorResponse)
		if err != nil {
			return "", errors.Wrap(err, "Failed to read publish notification response due to invalid JSON")
		}

		errorMessage := fmt.Sprintf("%s: %s", pubErrorResponse.Error, pubErrorResponse.Description)
		return "", errors.Wrap(errors.New(errorMessage), "Failed to publish notification")
	}
}
