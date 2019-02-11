package twilio

import (
	"encoding/xml"
	"testing"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/flows/waits"
	"github.com/nyaruka/goflow/flows/waits/hints"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/config"

	"github.com/nyaruka/goflow/flows"
	"github.com/stretchr/testify/assert"
)

func TestResponseForSprint(t *testing.T) {
	// for tests it is more convenient to not have formatted output
	indentMarshal = false

	urn := urns.URN("tel:+12067799294")
	channelRef := assets.NewChannelReference(assets.ChannelUUID(utils.NewUUID()), "Twilio Channel")

	resumeURL := "http://temba.io/resume?session=1"

	// set our attachment domain for testing
	config.Mailroom.AttachmentDomain = "mailroom.io"
	defer func() { config.Mailroom.AttachmentDomain = "" }()

	tcs := []struct {
		Events   []flows.Event
		Wait     flows.Wait
		Expected string
	}{
		{
			[]flows.Event{events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "hello world", nil, nil))},
			nil,
			`<Response><Say>hello world</Say><Hangup></Hangup></Response>`,
		},
		{
			[]flows.Event{events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "hello world", []flows.Attachment{flows.Attachment("audio:/recordings/foo.wav")}, nil))},
			nil,
			`<Response><Play>https://mailroom.io/recordings/foo.wav</Play><Hangup></Hangup></Response>`,
		},
		{
			[]flows.Event{events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "hello world", []flows.Attachment{flows.Attachment("audio:https://temba.io/recordings/foo.wav")}, nil))},
			nil,
			`<Response><Play>https://temba.io/recordings/foo.wav</Play><Hangup></Hangup></Response>`,
		},
		{
			[]flows.Event{
				events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "hello world", nil, nil)),
				events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "goodbye", nil, nil)),
			},
			nil,
			`<Response><Say>hello world</Say><Say>goodbye</Say><Hangup></Hangup></Response>`,
		},
		{
			[]flows.Event{events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "enter a number", nil, nil))},
			waits.NewMsgWait(nil, hints.NewFixedDigitsHint(1)),
			`<Response><Gather numDigits="1" timeout="120" action="http://temba.io/resume?session=1&amp;wait_type=gather"><Say>enter a number</Say></Gather><Redirect>http://temba.io/resume?session=1&amp;wait_type=gather&amp;timeout=true</Redirect></Response>`,
		},
		{
			[]flows.Event{events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "enter a number, then press #", nil, nil))},
			waits.NewMsgWait(nil, hints.NewTerminatedDigitsHint("#")),
			`<Response><Gather finishOnKey="#" timeout="120" action="http://temba.io/resume?session=1&amp;wait_type=gather"><Say>enter a number, then press #</Say></Gather><Redirect>http://temba.io/resume?session=1&amp;wait_type=gather&amp;timeout=true</Redirect></Response>`,
		},
		{
			[]flows.Event{events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "say something", nil, nil))},
			waits.NewMsgWait(nil, hints.NewAudioHint()),
			`<Response><Say>say something</Say><Record action="http://temba.io/resume?session=1&amp;wait_type=record"></Record><Redirect>http://temba.io/resume?session=1&amp;wait_type=record&amp;empty=true</Redirect></Response>`,
		},
	}

	for i, tc := range tcs {
		response, err := responseForSprint(resumeURL, tc.Wait, tc.Events)
		assert.NoError(t, err, "%d: unexpected error")
		assert.Equal(t, xml.Header+tc.Expected, response, "%d: unexpected response", i)
	}
}