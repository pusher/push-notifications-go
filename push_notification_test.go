package pushnotifications

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	testInstanceId = "i-123"
	testSecretKey  = "k-456"
)

var (
	testPublishRequest = map[string]interface{}{
		"fcm": map[string]interface{}{
			"notification": map[string]interface{}{
				"title": "Hello",
				"body":  "Hello, world",
			},
		},
	}
)

func TestPushNotifications(t *testing.T) {
	Convey("A Push Notifications Instance", t, func() {
		Convey("should not be created if the Instance Id is an empty string", func() {
			noPN, err := New("", testSecretKey)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Instance Id can not be an empty string")
			So(noPN, ShouldBeNil)
		})

		Convey("should not be created if the Secret Key is an empty string", func() {
			noPN, err := New(testInstanceId, "")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Secret Key can not be an empty string")
			So(noPN, ShouldBeNil)
		})

		pn, noErrors := New(testInstanceId, testSecretKey)
		So(noErrors, ShouldBeNil)
		So(pn, ShouldNotBeNil)

		Convey("when publishing", func() {
			Convey("should fail if no interests are given", func() {
				pubId, err := pn.Publish([]string{}, testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err.Error(), ShouldContainSubstring, "No interests were supplied")
			})

			Convey("should fail if too many interests are given", func() {
				pubId, err := pn.Publish(make([]string, 9001), testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err.Error(), ShouldContainSubstring, "Too many interests supplied (9001): API only supports up to 10")
			})

			Convey("should fail if a zero-length interest is given", func() {
				pubId, err := pn.Publish([]string{"ok", ""}, testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err.Error(), ShouldContainSubstring, "An empty interest name is not valid")
			})

			Convey("should fail if a interest with a very long name is given", func() {
				longInterestName := ""
				for i := 0; i < 9001; i++ {
					longInterestName += "a"
				}

				pubId, err := pn.Publish([]string{longInterestName}, testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err.Error(), ShouldContainSubstring, "Interest length is 9001 which is over 164 characters")
			})

			Convey("should fail if it contains invalid chars", func() {
				pubId, err := pn.Publish([]string{`#not<>|ok`}, testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err.Error(), ShouldContainSubstring, "Interest `#not<>|ok` contains an forbidden character")
			})

			Convey("given a server it", func() {
				var lastHttpPayload []byte
				var serverRequestHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {} // no-op

				successHttpHandler := func(w http.ResponseWriter, r *http.Request) {
					lastHttpPayload, _ = ioutil.ReadAll(r.Body)
					serverRequestHandler(w, r)
				}
				testServer := httptest.NewServer(http.HandlerFunc(successHttpHandler))
				defer testServer.Close()

				pn.(*pushNotifications).baseEndpoint = testServer.URL

				Convey("should return an error if the server 400 Bad Request response and contains invalid JSON", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{bad-json"}`))
					}

					pubId, err := pn.Publish([]string{"hello"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err.Error(), ShouldContainSubstring, "invalid JSON")
				})

				Convey("should return an error if the server 400 Bad Request response", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error":"123","description":"why"}`))
					}

					pubId, err := pn.Publish([]string{"hello"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err.Error(), ShouldContainSubstring, "Failed to publish notification")
					So(err.Error(), ShouldContainSubstring, "123")
					So(err.Error(), ShouldContainSubstring, "why")
				})

				Convey("should return an network error if the request times-out", func() {
					pn.(*pushNotifications).httpClient.Timeout = time.Nanosecond
					pubId, err := pn.Publish([]string{"hello"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err.Error(), ShouldContainSubstring, "Failed")
				})

				Convey("should return an error if the server 200 OK response is invalid JSON", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{bad-json"}`))
					}

					pubId, err := pn.Publish([]string{"hello"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err.Error(), ShouldContainSubstring, "invalid JSON")
				})

				Convey("should return the publish id if the request is valid", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"publishId":"pub-123"}`))
					}

					pubId, err := pn.Publish([]string{"hell-o"}, testPublishRequest)
					So(pubId, ShouldEqual, "pub-123")
					So(err, ShouldBeNil)

					expected := `{"fcm":{"notification":{"body":"Hello, world","title":"Hello"}},"interests":["hell-o"]}`
					So(string(lastHttpPayload), ShouldResemble, expected)
				})
			})
		})

		Convey("when authenticating a User", func() {
			Convey("should return an error if the User Id is empty", func() {
				token, err := pn.AuthenticateUser("")

				So(err, ShouldNotBeNil)
				So(token, ShouldEqual, "")
			})

			Convey("should return an error if the User Id is too long", func() {
				s := ""
				for i := 0; i < maxUserIdLength; i++ {
					s += "a"
				}

				token, err := pn.AuthenticateUser(s)

				So(err, ShouldBeNil)
				So(token, ShouldNotEqual, "")

				token, err = pn.AuthenticateUser(s + "a")

				So(err, ShouldNotBeNil)
				So(token, ShouldEqual, "")
			})

			Convey("should return a valid JWT token if everything is correct", func() {
				token, err := pn.AuthenticateUser("u-123")

				So(err, ShouldBeNil)
				So(token, ShouldNotEqual, "")

				parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
					return []byte(testSecretKey), nil
				})

				So(err, ShouldBeNil)
				So(parsedToken, ShouldNotBeNil)
				So(parsedToken.Valid, ShouldBeTrue)
				So(parsedToken.Claims.(jwt.MapClaims)["sub"], ShouldEqual, "u-123")
			})
		})
	})
}
