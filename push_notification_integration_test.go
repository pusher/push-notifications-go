package pushnotifications

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// Integration tests that just confirm that the SDK can make valid requests to
// the backend. These are not meant to be exhaustive, merely to assert that
// the server understands the client and that the unit tests weren't written
// with the wrong assumptions.
func TestPushNotificationsWithServer(t *testing.T) {
	Convey("A Push Notifications Instance when talking to the actual server", t, func() {
		pn, err := New(
			"9aa32e04-a212-44ab-a592-9aeba66e46ac",
			"188C879D394E09FDECC04606A126FAE2125FEABD24A2D12C6AC969AE1CEE2AEC",
		)
		So(err, ShouldBeNil)

		var pubReq = map[string]interface{}{
			"fcm": map[string]interface{}{
				"notification": map[string]interface{}{
					"title": "Hello",
					"body":  "Hello, world",
				},
			},
		}

		Convey("should return no errors when deleting some user", func() {
			err := pn.DeleteUser("u-123")
			So(err, ShouldBeNil)
		})

		Convey("should return no errors when publishing to an user", func() {
			pubId, err := pn.PublishToUsers([]string{"u-123"}, pubReq)
			So(err, ShouldBeNil)
			So(pubId, ShouldNotBeEmpty)
		})

		Convey("should return no errors when publishing to an interest", func() {
			pubId, err := pn.PublishToInterests([]string{"i-123"}, pubReq)
			So(err, ShouldBeNil)
			So(pubId, ShouldNotBeEmpty)
		})
	})
}
