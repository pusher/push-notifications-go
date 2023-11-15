package pushnotifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"time"
	"unicode/utf8"

	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
)

// The Pusher Push Notifications Server API client
type PushNotifications interface {
	// Publishes notifications to all devices subscribed to at least 1 of the interests given
	// Returns a non-empty `publishId` JSON string if successful; or a non-nil `error` otherwise.
	PublishToInterests(interests []string, request map[string]interface{}) (publishId string, err error)

	// DEPRECATED. An alias for `PublishToInterests`
	Publish(interests []string, request map[string]interface{}) (publishId string, err error)

	// Publishes notifications to all devices associated with the given user ids
	// Returns a non-empty `publishId` JSON string successful, or a non-nil `error` otherwise.
	PublishToUsers(users []string, request map[string]interface{}) (publishId string, err error)

	// Creates a signed JWT for a user id.
	// Returns a signed JWT if successful, or a non-nil `error` otherwise.
	GenerateToken(userId string) (token map[string]interface{}, err error)

	// Contacts the Beams service to remove all the devices of the given user
	// Return a non-nil `error` if there's a problem.
	DeleteUser(userId string) (err error)
}

const (
	defaultRequestTimeout       = time.Minute
	defaultBaseEndpointFormat   = "https://%s.pushnotifications.pusher.com"
	maxUserIdLength             = 164
	maxNumUserIdsWhenPublishing = 1000
	tokenTTL                    = 24 * time.Hour
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
func New(instanceId string, secretKey string, options ...Option) (PushNotifications, error) {
	if instanceId == "" {
		return nil, errors.New("Instance Id cannot be an empty string")
	}
	if secretKey == "" {
		return nil, errors.New("Secret Key cannot be an empty string")
	}

	pn := &pushNotifications{
		InstanceId: instanceId,
		SecretKey:  secretKey,

		baseEndpoint: fmt.Sprintf(defaultBaseEndpointFormat, instanceId),
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}

	for _, option := range options {
		option(pn)
	}

	return pn, nil
}

type publishResponse struct {
	PublishId string `json:"publishId"`
}

type errorResponse struct {
	Error       string `json:"error"`
	Description string `json:"description"`
}

func (pn *pushNotifications) GenerateToken(userId string) (map[string]interface{}, error) {
	if len(userId) == 0 {
		return nil, errors.New("User Id cannot be empty")
	}

	if len(userId) > maxUserIdLength {
		return nil, errors.Errorf(
			"User Id ('%s') length too long (expected fewer than %d characters, got %d)",
			userId, maxUserIdLength+1, len(userId))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userId,
		"exp": time.Now().Add(tokenTTL).Unix(),
		"iss": "https://" + pn.InstanceId + ".pushnotifications.pusher.com",
	})

	tokenString, signingErrorErr := token.SignedString([]byte(pn.SecretKey))
	if signingErrorErr != nil {
		return nil, errors.Wrap(signingErrorErr, "Failed to sign the JWT token used for User Authentication")
	}

	tokenMap := map[string]interface{}{
		"token": tokenString,
	}

	return tokenMap, nil
}

// Deprecated: Use PublishToInterests instead
func (pn *pushNotifications) Publish(interests []string, request map[string]interface{}) (string, error) {
	return pn.PublishToInterests(interests, request)
}

func (pn *pushNotifications) PublishToInterests(interests []string, request map[string]interface{}) (string, error) {
	if len(interests) == 0 {
		// this request was not very interesting :/
		return "", errors.New("No interests were supplied")
	}

	if len(interests) > 100 {
		return "",
			errors.Errorf("Too many interests supplied (%d): API only supports up to 100", len(interests))
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
	// TODO: don't mutate `request`
	request["interests"] = interests
	bodyRequestBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "Failed to marshal the publish request JSON body")
	}

	URL := fmt.Sprintf(pn.baseEndpoint+"/publish_api/v1/instances/%s/publishes", pn.InstanceId)
	return pn.publishToAPI(URL, bodyRequestBytes)
}

func (pn *pushNotifications) PublishToUsers(users []string, request map[string]interface{}) (string, error) {
	if len(users) == 0 {
		return "", errors.New("Must supply at least one user id")
	}
	if len(users) > maxNumUserIdsWhenPublishing {
		return "", errors.New(
			fmt.Sprintf("Too many user ids supplied. API supports up to %d, got %d", maxNumUserIdsWhenPublishing, len(users)),
		)
	}
	for i, userId := range users {
		if userId == "" {
			return "", errors.New("Empty user ids are not valid")
		}
		if len(userId) > maxUserIdLength {
			return "", errors.New(
				fmt.Sprintf("User Id ('%s') length too long (expected fewer than %d characters, got %d)", userId, maxUserIdLength, len(userId)),
			)
		}
		// test for invalid characters
		if !utf8.ValidString(userId) {
			return "", errors.New(fmt.Sprintf("User Id at index %d is not valid utf8", i))
		}
	}
	// TODO: don't mutate `request`
	request["users"] = users
	bodyRequestBytes, err := json.Marshal(request)
	if err != nil {
		return "", errors.Wrap(err, "Failed to marshal the publish request JSON body")
	}

	URL := fmt.Sprintf("%s/publish_api/v1/instances/%s/publishes/users", pn.baseEndpoint, pn.InstanceId)
	return pn.publishToAPI(URL, bodyRequestBytes)
}

func (pn *pushNotifications) publishToAPI(url string, bodyRequestBytes []byte) (string, error) {
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
		pubErrorResponse := &errorResponse{}
		err = json.Unmarshal(responseBytes, pubErrorResponse)
		if err != nil {
			return "", errors.Wrap(err, "Failed to read publish notification response due to invalid JSON")
		}

		errorMessage := fmt.Sprintf("%s: %s", pubErrorResponse.Error, pubErrorResponse.Description)
		return "", errors.Wrap(errors.New(errorMessage), "Failed to publish notification")
	}
}

func (pn *pushNotifications) DeleteUser(userId string) error {
	if len(userId) == 0 {
		return errors.New("User Id cannot be empty")
	}

	if len(userId) > maxUserIdLength {
		return errors.Errorf(
			"User Id ('%s') length too long (expected fewer than %d characters, got %d)",
			userId, maxUserIdLength+1, len(userId))
	}

	if !utf8.ValidString(userId) {
		return errors.New("User Id must be encoded using utf8")
	}

	URL := fmt.Sprintf("%s/customer_api/v1/instances/%s/users/%s", pn.baseEndpoint, pn.InstanceId, url.PathEscape(userId))
	httpReq, err := http.NewRequest(http.MethodDelete, URL, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to prepare the delete user request")
	}

	httpReq.Header.Add("Authorization", "Bearer "+pn.SecretKey)
	httpReq.Header.Add("Content-Type", "application/json")
	httpReq.Header.Add("X-Pusher-Library", "pusher-push-notifications-go "+sdkVersion)

	httpResp, err := pn.httpClient.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "Failed to delete user due to a network error")
	}

	defer httpResp.Body.Close()
	responseBytes, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return errors.Wrap(err, "Failed to read delete user response due to a network error")
	}

	switch httpResp.StatusCode {
	case http.StatusOK:
		return nil
	default:
		errResponse := &errorResponse{}
		err = json.Unmarshal(responseBytes, errResponse)
		if err != nil {
			return errors.Wrap(err, "Failed to read delete user response due to invalid JSON")
		}

		errorMessage := fmt.Sprintf("%s: %s", errResponse.Error, errResponse.Description)
		return errors.Wrap(errors.New(errorMessage), "Failed to delete user")
	}

	return nil
}
