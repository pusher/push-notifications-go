package pushnotifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

// The Pusher Push Notifications Server API client
type PushNotifications interface {
	// Publishes notifications to all devices subscribed to at least 1 of the interests given
	// Returns a non-empty `publishId` string if successful; or a non-nil `error` otherwise.
	Publish(interests []string, request map[string]interface{}) (publishId string, err error)
	AuthenticateUser(userId string) (string, error)
}

const (
	defaultRequestTimeout     = time.Minute
	defaultBaseEndpointFormat = "https://%s.pushnotifications.pusher.com"
	maxUserIdLength           = 255
)

var (
	interestValidationRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-=@,.;]+$`)
)

type pushNotifications struct {
	InstanceId string
	SecretKey  string

	baseEndpoint string
	httpClient   *http.Client
}

// Creates a New `PushNotifications` instance.
// Returns an non-nil error if `instanceId` or `secretKey` are empty
func New(instanceId string, secretKey string) (PushNotifications, error) {
	if instanceId == "" {
		return nil, errors.New("Instance Id can not be an empty string")
	}
	if secretKey == "" {
		return nil, errors.New("Secret Key can not be an empty string")
	}

	return &pushNotifications{
		InstanceId: instanceId,
		SecretKey:  secretKey,

		baseEndpoint: fmt.Sprintf(defaultBaseEndpointFormat, instanceId),
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}, nil
}

type publishResponse struct {
	PublishId string `json:"publishId"`
}

type publishErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"description"`
}

func (pn *pushNotifications) AuthenticateUser(userId string) (string, error) {
	if len(userId) == 0 {
		return "", errors.New("User Id can not be empty")
	}

	if len(userId) > maxUserIdLength {
		return "", errors.Errorf(
			"User Id ('%s') length too long (expected less than %d, got %d)",
			userId, maxUserIdLength+1, len(userId))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userId,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iss": "https://" + pn.InstanceId + ".pushnotifications.pusher.com",
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, signingErrorErr := token.SignedString([]byte(pn.SecretKey))
	if signingErrorErr != nil {
		return "", errors.Wrap(signingErrorErr, "Failed to sign the JWT token used for User Authentication")
	}

	return tokenString, nil
}

func (pn *pushNotifications) Publish(interests []string, request map[string]interface{}) (string, error) {
	if len(interests) == 0 {
		// this request was not very interesting :/
		return "", errors.New("No interests were supplied")
	}

	if len(interests) > 10 {
		return "",
			errors.Errorf("Too many interests supplied (%d): API only supports up to 10", len(interests))
	}

	for _, interest := range interests {
		if len(interest) == 0 {
			return "", errors.New("An empty interest name is not valid")
		}

		if len(interest) > 164 {
			return "",
				errors.Errorf("Interest length is %d which is over 164 characters", len(interest))
		}

		if !interestValidationRegex.MatchString(interest) {
			return "",
				errors.Errorf(
					"Interest `%s` contains an forbidden character: "+
						"Allowed characters are: ASCII upper/lower-case letters, "+
						"numbers or one of _-=@,.:",
					interest)
		}
	}

	request["interests"] = interests
	bodyRequestBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "Failed to marshal the publish request JSON body")
	}

	url := fmt.Sprintf(pn.baseEndpoint+"/publish_api/v1/instances/%s/publishes", pn.InstanceId)

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyRequestBytes))
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
