package conversion

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/dialogs/dialog-push-service/pkg/api"
	"github.com/dialogs/dialog-push-service/pkg/provider/ans"
	"github.com/pkg/errors"
	"github.com/sideshow/apns2/payload"
)

func RequestPbToAns(in *api.PushBody, supportsVoIP, allowAlerts bool, topic, sound *string) (*ans.Request, error) {

	var (
		out ans.Request
		err error
	)

	payload := payload.NewPayload()
	if voip := in.GetVoipPush(); voip != nil {
		err = setVoIPPayloadAns(payload, voip, supportsVoIP)

	} else if alerting := in.GetAlertingPush(); alerting != nil {
		setAlertingPayloadAns(payload, alerting, sound, allowAlerts)

	} else if encryped := in.GetEncryptedPush(); encryped != nil {
		err = setEncryptedPayload(payload, encryped, sound)

	} else if silent := in.GetSilentPush(); silent != nil {
		// ignoring
		return nil, nil

	} else {
		err = ErrorByIncomingMessage(in)

	}

	if err != nil {
		return nil, err
	}

	if seq := in.GetSeq(); seq > 0 {
		payload.Custom("seq", seq)
	}

	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return nil, err
	}

	if out.Headers.Expiration.Truncate(time.Hour).IsZero() {
		out.Headers.Expiration = time.Now().Add(20 * time.Minute) // TODO: to settings or task.body.TimeToLive?
	}

	if id := in.GetCollapseKey(); id != "" {
		out.Headers.CollapseID = id
	}

	if topic != nil && *topic != "" {
		out.Headers.Topic = *topic
	}

	out.Payload = buf.Bytes()

	return &out, nil
}

func setVoIPPayloadAns(payload *payload.Payload, src *api.VoipPush, supportsVoIP bool) error {

	if !supportsVoIP {
		return errors.New("attempted voip-push using non-voip certificate")
	}

	payload.Custom("callId", strconv.Itoa(int(src.GetCallId())))
	payload.Custom("callIdStr", src.GetCallIdStr())
	payload.Custom("attemptIndex", src.GetAttemptIndex())
	payload.Custom("displayName", src.GetDisplayName())
	payload.Custom("eventBusId", src.GetEventBusId())
	payload.Custom("updateType", src.GetUpdateType())
	payload.Custom("disposalReason", src.GetDisposalReason())
	payload.Custom("video", src.GetVideo())
	payload.Custom("token", src.GetToken())
	payload.Custom("endpoint", src.GetEndpoint())

	if peer := src.GetPeer(); peer != nil {
		peerInfo := map[string]string{
			"id":    strconv.Itoa(int(peer.Id)),
			"type":  strconv.Itoa(PeerTypeProtobufToMPS(peer.Type)),
			"strId": peer.StrId,
		}
		payload.Custom("peer", peerInfo)
	}

	if outPeer := src.GetOutPeer(); outPeer != nil {
		peerInfo := map[string]string{
			"id":         strconv.Itoa(int(outPeer.Id)),
			"type":       strconv.Itoa(PeerTypeProtobufToMPS(outPeer.Type)),
			"accessHash": strconv.Itoa(int(outPeer.AccessHash)),
			"strId":      outPeer.StrId,
		}
		payload.Custom("outPeer", peerInfo)
	}

	if merge := src.GetMerge(); merge != nil {
		mergeInfo := map[string]string{
			"key": merge.GetKey(),
			"merge": strconv.FormatBool(merge.GetMerge()),
		}
		payload.Custom("merge", mergeInfo)
	}

	return nil
}

func setAlertingPayloadAns(payload *payload.Payload, src *api.AlertingPush, sound *string, allowAlerts bool) {

	if allowAlerts {
		setAlertPropsAns(payload, src, sound)
		payload.MutableContent()

		if category := src.Category; category != nil {
			payload.Custom("category", category.Value)
		}

	} else {
		// alerting pushes are disabled, sending silent instead
		if badge := src.GetBadge(); badge > 0 {
			payload.Badge(int(badge))
		}

		payload.ContentAvailable()
		payload.Sound("")

	}
}

func setEncryptedPayload(payload *payload.Payload, src *api.EncryptedPush, sound *string) error {

	if public := src.GetPublicAlertingPush(); public != nil {
		setAlertPropsAns(payload, public, sound)
	}

	encryptedData := src.GetEncryptedData()
	if len(encryptedData) == 0 {
		return ErrEmptyEncryptedPayload
	}

	userInfo := map[string]string{
		"nonce":          strconv.Itoa(int(src.Nonce)),
		"encrypted_data": base64.StdEncoding.EncodeToString(encryptedData),
	}

	payload.MutableContent()
	payload.Custom("user_info", userInfo)

	return nil
}

func setAlertPropsAns(payload *payload.Payload, alerting *api.AlertingPush, sound *string) {

	if locAlert := alerting.GetLocAlertTitle(); locAlert != nil {
		payload.AlertTitleLocKey(locAlert.GetLocKey())
		payload.AlertTitleLocArgs(locAlert.GetLocArgs())

	} else if simpleTitle := alerting.GetSimpleAlertTitle(); len(simpleTitle) > 0 {
		payload.AlertTitle(simpleTitle)

	}

	if locBody := alerting.GetLocAlertBody(); locBody != nil {
		payload.AlertLocKey(locBody.GetLocKey())
		payload.AlertLocArgs(locBody.GetLocArgs())

	} else if simpleBody := alerting.GetSimpleAlertBody(); len(simpleBody) > 0 {
		payload.AlertBody(simpleBody)

	}

	if sound != nil && len(*sound) > 0 {
		payload.Sound(*sound)
	}

	if badge := alerting.GetBadge(); badge > 0 {
		payload.Badge(int(badge))
	}
}
