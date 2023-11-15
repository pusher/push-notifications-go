package pushnotifications

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
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
			So(err.Error(), ShouldContainSubstring, "Instance Id cannot be an empty string")
			So(noPN, ShouldBeNil)
		})

		Convey("should not be created if the Secret Key is an empty string", func() {
			noPN, err := New(testInstanceId, "")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Secret Key cannot be an empty string")
			So(noPN, ShouldBeNil)
		})

		pn, noErrors := New(testInstanceId, testSecretKey)
		So(noErrors, ShouldBeNil)
		So(pn, ShouldNotBeNil)

		Convey("when publishing to interests", func() {
			functions := map[string]func(interests []string, request map[string]interface{}) (publishId string, err error){
				"PublishToInterests": pn.PublishToInterests,
				"Publish":            pn.Publish, // this is a deprecated alias
			}

			for funcName, publishToInterests := range functions {
				Convey("using `"+funcName+"`, it", func() {
					Convey("should fail if no interests are given", func() {
						pubId, err := publishToInterests([]string{}, testPublishRequest)
						So(pubId, ShouldEqual, "")
						So(err.Error(), ShouldContainSubstring, "No interests were supplied")
					})

					Convey("should fail if too many interests are given", func() {
						pubId, err := publishToInterests(make([]string, 9001), testPublishRequest)
						So(pubId, ShouldEqual, "")
						So(err.Error(), ShouldContainSubstring, "Too many interests supplied (9001): API only supports up to 10")
					})

					Convey("should fail if a zero-length interest is given", func() {
						pubId, err := publishToInterests([]string{"ok", ""}, testPublishRequest)
						So(pubId, ShouldEqual, "")
						So(err.Error(), ShouldContainSubstring, "An empty interest name is not valid")
					})

					Convey("should fail if a interest with a very long name is given", func() {
						longInterestName := ""
						for i := 0; i < 9001; i++ {
							longInterestName += "a"
						}

						pubId, err := publishToInterests([]string{longInterestName}, testPublishRequest)
						So(pubId, ShouldEqual, "")
						So(err.Error(), ShouldContainSubstring, "Interest length is 9001 which is over 164 characters")
					})

					Convey("should fail if it contains invalid chars", func() {
						pubId, err := publishToInterests([]string{`#not<>|ok`}, testPublishRequest)
						So(pubId, ShouldEqual, "")
						So(err.Error(), ShouldContainSubstring, "Interest `#not<>|ok` contains an forbidden character")
					})

					Convey("should fail if 101 interests are given", func() {
						interests := make([]string, 101)

						for i := range interests {
							interests[i] = fmt.Sprintf("%s", strconv.Itoa(i))
						}
						pubId, err := pn.Publish(interests, testPublishRequest)

						So(pubId, ShouldEqual, "")
						So(err.Error(), ShouldContainSubstring, "Too many interests")
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

							pubId, err := publishToInterests([]string{"hello"}, testPublishRequest)
							So(pubId, ShouldEqual, "")
							So(err.Error(), ShouldContainSubstring, "invalid JSON")
						})

						Convey("should return an error if the server responds with 400 Bad Request", func() {
							serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(http.StatusBadRequest)
								w.Write([]byte(`{"error":"123","description":"why"}`))
							}

							pubId, err := publishToInterests([]string{"hello"}, testPublishRequest)
							So(pubId, ShouldEqual, "")
							So(err, ShouldNotBeNil)
							So(err.Error(), ShouldContainSubstring, "Failed to publish notification")
							So(err.Error(), ShouldContainSubstring, "123")
							So(err.Error(), ShouldContainSubstring, "why")
						})

						Convey("should return an error if the server 200 OK response is invalid JSON", func() {
							serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(http.StatusOK)
								w.Write([]byte(`{bad-json"}`))
							}

							pubId, err := publishToInterests([]string{"hello"}, testPublishRequest)
							So(pubId, ShouldEqual, "")
							So(err.Error(), ShouldContainSubstring, "invalid JSON")
						})

						Convey("should return the publish id if the request is valid", func() {
							serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(http.StatusOK)
								w.Write([]byte(`{"publishId":"pub-123"}`))
							}

							pubId, err := publishToInterests([]string{"hell-o"}, testPublishRequest)
							So(pubId, ShouldEqual, "pub-123")
							So(err, ShouldBeNil)

							expected := `{"fcm":{"notification":{"body":"Hello, world","title":"Hello"}},"interests":["hell-o"]}`
							So(string(lastHttpPayload), ShouldResemble, expected)
						})
					})

					Convey("given a slow server, it", func() {
						slowHttpHandler := func(w http.ResponseWriter, r *http.Request) {
							time.Sleep(200 * time.Millisecond)
						}
						testServer := httptest.NewServer(http.HandlerFunc(slowHttpHandler))
						defer testServer.Close()

						pn.(*pushNotifications).baseEndpoint = testServer.URL

						Convey("should return a network error if the request times out", func() {
							pn.(*pushNotifications).httpClient.Timeout = time.Nanosecond
							pubId, err := publishToInterests([]string{"hello"}, testPublishRequest)
							So(pubId, ShouldEqual, "")
							So(err, ShouldNotBeNil)
							So(err.Error(), ShouldContainSubstring, "Failed")
						})

					})
				})
			}
		})

		Convey("when generating a token", func() {
			Convey("should return an error if the User Id is empty", func() {
				token, err := pn.GenerateToken("")

				So(err, ShouldNotBeNil)
				So(token, ShouldBeNil)
				So(err.Error(), ShouldContainSubstring, "User Id cannot be empty")
			})

			Convey("should return an error if the User Id is too long", func() {
				s := ""
				for i := 0; i < maxUserIdLength; i++ {
					s += "a"
				}

				token, err := pn.GenerateToken(s)

				So(err, ShouldBeNil)
				So(token, ShouldNotBeNil)

				longerUserId := s + "a"
				token, err = pn.GenerateToken(s + "a")

				So(err, ShouldNotBeNil)
				So(token, ShouldBeNil)
				So(
					err.Error(),
					ShouldContainSubstring,
					fmt.Sprintf("User Id ('%s') length too long (expected fewer than %d characters, got %d)", longerUserId, maxUserIdLength+1, len(longerUserId)),
				)
			})

			Convey("should return a valid JWT token if everything is correct", func() {
				tokenMap, err := pn.GenerateToken("u-123")

				token := tokenMap["token"].(string)
				So(err, ShouldBeNil)
				So(token, ShouldNotEqual, "")

				expectedIssuer := "https://" + testInstanceId + ".pushnotifications.pusher.com"
				expectedSubject := "u-123"

				parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
					return []byte(testSecretKey), nil
				})
				So(err, ShouldBeNil)

				So(parsedToken, ShouldNotEqual, jwt.Token{})
				So(parsedToken.Valid, ShouldBeTrue)

				So(parsedToken.Claims.(jwt.MapClaims)["iss"], ShouldEqual, expectedIssuer)
				So(parsedToken.Claims.(jwt.MapClaims)["sub"], ShouldEqual, expectedSubject)
				expirySeconds := parsedToken.Claims.(jwt.MapClaims)["exp"]
				expiry := expirySeconds.(float64)
				So(time.Unix(int64(expiry), 0), ShouldHappenAfter, time.Now())
			})
		})

		Convey("when publishing to Users", func() {
			Convey("should fail if no Users are given", func() {
				pubId, err := pn.PublishToUsers([]string{}, testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "Must supply at least one user id")
			})

			Convey("should fail if too many Users are given", func() {
				pubId, err := pn.PublishToUsers(make([]string, 1001), testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, fmt.Sprintf("Too many user ids supplied. API supports up to %d, got %d", maxNumUserIdsWhenPublishing, 1001))
			})

			Convey("should fail if a zero-length User id is given", func() {
				pubId, err := pn.PublishToUsers(make([]string, 5), testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "Empty user ids are not valid")
			})

			Convey("should fail if a User id is too long", func() {
				var tooLong string
				for i := 0; i < maxUserIdLength+1; i++ {
					tooLong += "h"
				}
				pubId, err := pn.PublishToUsers([]string{"a", "b", tooLong, "d"}, testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err, ShouldNotBeNil)
				So(
					err.Error(),
					ShouldContainSubstring,
					fmt.Sprintf("User Id ('%s') length too long (expected fewer than %d characters, got %d)", tooLong, maxUserIdLength, len(tooLong)),
				)
			})

			Convey("should fail if a User id contains invalid chars", func() {
				invalid := []byte{192}
				pubId, err := pn.PublishToUsers([]string{"a", "b", string(invalid), "d"}, testPublishRequest)
				So(pubId, ShouldEqual, "")
				So(err, ShouldNotBeNil)
				So(
					err.Error(),
					ShouldContainSubstring,
					fmt.Sprintf("User Id at index %d is not valid utf8", 2),
				)
			})

			Convey("given a server, it", func() {
				var lastHttpPayload []byte
				var serverRequestHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {} // no-op

				successHttpHandler := func(w http.ResponseWriter, r *http.Request) {
					lastHttpPayload, _ = ioutil.ReadAll(r.Body)
					serverRequestHandler(w, r)
				}
				testServer := httptest.NewServer(http.HandlerFunc(successHttpHandler))
				defer testServer.Close()

				pn.(*pushNotifications).baseEndpoint = testServer.URL

				Convey("should return an error if the server returns a 400 Bad Request response and contains invalid JSON", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{bad-json"}`))
					}

					pubId, err := pn.PublishToUsers([]string{"user-id-1"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "invalid JSON")
				})

				Convey("should return an error if the server responds with 400 Bad Request", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error": "123", "description": "a lovely description"}`))
					}

					pubId, err := pn.PublishToUsers([]string{"user-id-1"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "Failed to publish notification")
					So(err.Error(), ShouldContainSubstring, "123")
					So(err.Error(), ShouldContainSubstring, "a lovely description")
				})

				Convey("should return an error if the server responds 200 OK but returns invalid JSON", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{bad-json"}`))
					}

					pubId, err := pn.PublishToUsers([]string{"user-id-1"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "invalid JSON")
				})

				Convey("should return the publish id if the request is valid", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"publishId": "pub-123"}`))

						expectedHttpPayload := `
						{
							"fcm": {
								"notification": {
									"body": "Hello, world",
									"title": "Hello",
									"users":["user-1"]
								}
							}
						}
						`

						pubId, err := pn.PublishToUsers([]string{"user-id-1"}, testPublishRequest)
						So(pubId, ShouldEqual, "pub-123")
						So(err, ShouldBeNil)
						So(string(lastHttpPayload), ShouldResemble, expectedHttpPayload)
					}
				})

				Convey("should succeed if 100 interests are given", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"publishId":"pub-123"}`))
					}

					interests := make([]string, 100)

					for i := range interests {
						interests[i] = fmt.Sprintf("%s", strconv.Itoa(i))
					}
					pubId, err := pn.Publish(interests, testPublishRequest)

					So(pubId, ShouldNotBeNil)
					So(err, ShouldBeNil)
				})
			})

			Convey("given a slow server, it", func() {
				slowHttpHandler := func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(200 * time.Millisecond)
				}
				testServer := httptest.NewServer(http.HandlerFunc(slowHttpHandler))
				defer testServer.Close()

				pn.(*pushNotifications).baseEndpoint = testServer.URL

				Convey("should return a network error if the request times out", func() {
					pn.(*pushNotifications).httpClient.Timeout = time.Nanosecond
					pubId, err := pn.PublishToUsers([]string{"user-id-1"}, testPublishRequest)
					So(pubId, ShouldEqual, "")
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "Failed to publish notifications due to a network error")
				})
			})
		})

		Convey("when deleting a User", func() {
			Convey("should fail if no User is given", func() {
				err := pn.DeleteUser("")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "User Id cannot be empty")
			})

			Convey("should return an error if the User Id is too long", func() {
				s := ""
				for i := 0; i < maxUserIdLength; i++ {
					s += "a"
				}

				err := pn.DeleteUser(s + "a")

				So(err, ShouldNotBeNil)
				So(
					err.Error(),
					ShouldContainSubstring,
					fmt.Sprintf("User Id ('%s') length too long (expected fewer than %d characters, got %d)", s+"a", maxUserIdLength+1, len(s)+1),
				)
			})

			Convey("should fail if a User id contains invalid chars", func() {
				invalid := []byte{192}
				err := pn.DeleteUser(string(invalid))
				So(err, ShouldNotBeNil)
				So(
					err.Error(),
					ShouldContainSubstring,
					"User Id must be encoded using utf8",
				)
			})

			Convey("given a server, it", func() {
				var lastHttpPayload []byte
				var serverRequestHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {} // no-op

				successHttpHandler := func(w http.ResponseWriter, r *http.Request) {
					lastHttpPayload, _ = ioutil.ReadAll(r.Body)
					serverRequestHandler(w, r)
				}
				testServer := httptest.NewServer(http.HandlerFunc(successHttpHandler))
				defer testServer.Close()

				pn.(*pushNotifications).baseEndpoint = testServer.URL

				Convey("should return an error if the server returns a 400 Bad Request response and contains invalid JSON", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{bad-json"}`))
					}

					err := pn.DeleteUser("user-id-1")
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "invalid JSON")
				})

				Convey("should return an error if the server responds with 400 Bad Request", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error": "123", "description": "a lovely description"}`))
					}
					err := pn.DeleteUser("user-id-1")
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "Failed to delete user")
					So(err.Error(), ShouldContainSubstring, "123")
					So(err.Error(), ShouldContainSubstring, "a lovely description")
				})

				Convey("should succeed if the request is valid", func() {
					serverRequestHandler = func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)

						expectedHttpPayload := ""

						err := pn.DeleteUser("user-id-1")
						So(err, ShouldBeNil)
						So(string(lastHttpPayload), ShouldResemble, expectedHttpPayload)
					}
				})
			})

			Convey("given a slow server, it", func() {
				slowHttpHandler := func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(200 * time.Millisecond)
				}
				testServer := httptest.NewServer(http.HandlerFunc(slowHttpHandler))
				defer testServer.Close()

				pn.(*pushNotifications).baseEndpoint = testServer.URL

				Convey("should return a network error if the request times out", func() {
					pn.(*pushNotifications).httpClient.Timeout = time.Nanosecond
					err := pn.DeleteUser("user-id-1")
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "Failed to delete user due to a network error")
				})
			})
		})
	})
}
