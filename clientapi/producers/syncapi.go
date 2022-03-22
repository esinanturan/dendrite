// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package producers

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// SyncAPIProducer produces events for the sync API server to consume
type SyncAPIProducer struct {
	TopicClientData   string
	TopicReceiptEvent string
	JetStream         nats.JetStreamContext
}

// SendData sends account data to the sync API server
func (p *SyncAPIProducer) SendData(userID string, roomID string, dataType string, readMarker *eventutil.ReadMarkerJSON) error {
	m := &nats.Msg{
		Subject: p.TopicClientData,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)

	data := eventutil.AccountData{
		RoomID:     roomID,
		Type:       dataType,
		ReadMarker: readMarker,
	}
	var err error
	m.Data, err = json.Marshal(data)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   roomID,
		"data_type": dataType,
	}).Tracef("Producing to topic '%s'", p.TopicClientData)

	_, err = p.JetStream.PublishMsg(m)
	return err
}

func (p *SyncAPIProducer) SendReceipt(
	ctx context.Context,
	userID, roomID, eventID, receiptType string, timestamp gomatrixserverlib.Timestamp,
) error {
	m := &nats.Msg{
		Subject: p.TopicReceiptEvent,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)
	m.Header.Set(jetstream.RoomID, roomID)
	m.Header.Set(jetstream.EventID, eventID)
	m.Header.Set("type", receiptType)
	m.Header.Set("timestamp", strconv.Itoa(int(timestamp)))

	log.WithFields(log.Fields{}).Tracef("Producing to topic '%s'", p.TopicReceiptEvent)
	_, err := p.JetStream.PublishMsg(m, nats.Context(ctx))
	return err
}
