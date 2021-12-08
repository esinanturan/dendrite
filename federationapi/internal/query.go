package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// QueryJoinedHostServerNamesInRoom implements api.FederationInternalAPI
func (f *FederationInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) (err error) {
	joinedHosts, err := f.db.GetJoinedHostsForRooms(ctx, []string{request.RoomID})
	if err != nil {
		return
	}
	response.ServerNames = joinedHosts

	return
}

func (f *FederationInternalAPI) QueryEventAuthFromFederation(
	ctx context.Context,
	request *api.QueryEventAuthFromFederationRequest,
	response *api.QueryEventAuthFromFederationResponse,
) error {
	joinedHosts, err := f.db.GetJoinedHostsForRooms(ctx, []string{request.RoomID})
	if err != nil {
		return fmt.Errorf("f.db.GetJoinedHostsForRooms: %w", err)
	}

	tryHost := func(serverName gomatrixserverlib.ServerName) error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()
		ires, err := f.doRequest(serverName, func() (interface{}, error) {
			return f.federation.GetEventAuth(
				ctx,
				serverName,
				request.RoomID,
				request.EventID,
			)
		})
		if err != nil {
			return fmt.Errorf("f.doRequest: %w", err)
		}
		tx := ires.(gomatrixserverlib.RespEventAuth)
		response.Events = tx.AuthEvents
		return nil
	}

	var lasterr error
	for _, host := range joinedHosts {
		if lasterr = tryHost(host); lasterr != nil {
			continue
		}
		break
	}

	return lasterr
}

func (f *FederationInternalAPI) QueryStateIDsFromFederation(
	ctx context.Context,
	request *api.QueryStateIDsFromFederationRequest,
	response *api.QueryStateIDsFromFederationResponse,
) error {
	joinedHosts, err := f.db.GetJoinedHostsForRooms(ctx, []string{request.RoomID})
	if err != nil {
		return fmt.Errorf("f.db.GetJoinedHostsForRooms: %w", err)
	}

	tryHost := func(serverName gomatrixserverlib.ServerName) error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()
		ires, err := f.doRequest(serverName, func() (interface{}, error) {
			return f.federation.LookupStateIDs(
				ctx,
				serverName,
				request.RoomID,
				request.EventID,
			)
		})
		if err != nil {
			return fmt.Errorf("f.doRequest: %w", err)
		}
		tx := ires.(gomatrixserverlib.RespStateIDs)
		response.AuthEventIDs = tx.AuthEventIDs
		response.StateEventIDs = tx.StateEventIDs
		return nil
	}

	var lasterr error
	for _, host := range joinedHosts {
		if lasterr = tryHost(host); lasterr != nil {
			continue
		}
		break
	}

	return lasterr
}

func (f *FederationInternalAPI) QueryStateFromFederation(
	ctx context.Context,
	request *api.QueryStateFromFederationRequest,
	response *api.QueryStateFromFederationResponse,
) error {
	joinedHosts, err := f.db.GetJoinedHostsForRooms(ctx, []string{request.RoomID})
	if err != nil {
		return fmt.Errorf("f.db.GetJoinedHostsForRooms: %w", err)
	}

	tryHost := func(serverName gomatrixserverlib.ServerName) error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()
		ires, err := f.doRequest(serverName, func() (interface{}, error) {
			return f.federation.LookupState(
				ctx,
				serverName,
				request.RoomID,
				request.EventID,
				request.RoomVersion,
			)
		})
		if err != nil {
			return fmt.Errorf("f.doRequest: %w", err)
		}
		tx := ires.(gomatrixserverlib.RespState)
		response.AuthEvents = tx.AuthEvents
		response.StateEvents = tx.StateEvents
		return nil
	}

	var lasterr error
	for _, host := range joinedHosts {
		if lasterr = tryHost(host); lasterr != nil {
			continue
		}
		break
	}

	return lasterr
}

func (a *FederationInternalAPI) fetchServerKeysDirectly(ctx context.Context, serverName gomatrixserverlib.ServerName) (*gomatrixserverlib.ServerKeys, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(serverName, func() (interface{}, error) {
		return a.federation.GetServerKeys(ctx, serverName)
	})
	if err != nil {
		return nil, err
	}
	sks := ires.(gomatrixserverlib.ServerKeys)
	return &sks, nil
}

func (a *FederationInternalAPI) fetchServerKeysFromCache(
	ctx context.Context, req *api.QueryServerKeysRequest,
) ([]gomatrixserverlib.ServerKeys, error) {
	var results []gomatrixserverlib.ServerKeys
	for keyID, criteria := range req.KeyIDToCriteria {
		serverKeysResponses, _ := a.db.GetNotaryKeys(ctx, req.ServerName, []gomatrixserverlib.KeyID{keyID})
		if len(serverKeysResponses) == 0 {
			return nil, fmt.Errorf("failed to find server key response for key ID %s", keyID)
		}
		// we should only get 1 result as we only gave 1 key ID
		sk := serverKeysResponses[0]
		util.GetLogger(ctx).Infof("fetchServerKeysFromCache: minvalid:%v  keys: %+v", criteria.MinimumValidUntilTS, sk)
		if criteria.MinimumValidUntilTS != 0 {
			// check if it's still valid. if they have the same value that's also valid
			if sk.ValidUntilTS < criteria.MinimumValidUntilTS {
				return nil, fmt.Errorf(
					"found server response for key ID %s but it is no longer valid, min: %v valid_until: %v",
					keyID, criteria.MinimumValidUntilTS, sk.ValidUntilTS,
				)
			}
		}
		results = append(results, sk)
	}
	return results, nil
}

func (a *FederationInternalAPI) QueryServerKeys(
	ctx context.Context, req *api.QueryServerKeysRequest, res *api.QueryServerKeysResponse,
) error {
	// attempt to satisfy the entire request from the cache first
	results, err := a.fetchServerKeysFromCache(ctx, req)
	if err == nil {
		// satisfied entirely from cache, return it
		res.ServerKeys = results
		return nil
	}
	util.GetLogger(ctx).WithField("server", req.ServerName).WithError(err).Warn("notary: failed to satisfy keys request entirely from cache, hitting direct")

	serverKeys, err := a.fetchServerKeysDirectly(ctx, req.ServerName)
	if err != nil {
		// try to load as much as we can from the cache in a best effort basis
		util.GetLogger(ctx).WithField("server", req.ServerName).WithError(err).Warn("notary: failed to ask server for keys, returning best effort keys")
		serverKeysResponses, dbErr := a.db.GetNotaryKeys(ctx, req.ServerName, req.KeyIDs())
		if dbErr != nil {
			return fmt.Errorf("notary: server returned %s, and db returned %s", err, dbErr)
		}
		res.ServerKeys = serverKeysResponses
		return nil
	}
	// cache it!
	if err = a.db.UpdateNotaryKeys(context.Background(), req.ServerName, *serverKeys); err != nil {
		// non-fatal, still return the response
		util.GetLogger(ctx).WithError(err).Warn("failed to UpdateNotaryKeys")
	}
	res.ServerKeys = []gomatrixserverlib.ServerKeys{*serverKeys}
	return nil
}