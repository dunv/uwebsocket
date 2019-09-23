package uwebsocket

import (
	"net/http"

	"github.com/dunv/ulog"
	"github.com/google/uuid"
)

var DemoHandler = func(hub *WebSocketHub) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		clientGUID := uuid.New()
		clientAttributeMap := map[string]interface{}{
			"clientGuid": clientGUID.String(),
		}

		// TODO: authentication
		// user, err := uauthHelpers.GetUserFromRequestGetParams(r, cfg.BCryptSecret)
		// if err == nil {
		// 	clientAttributeMap["userName"] = user.UserName
		// }

		ServeWs(hub, clientAttributeMap, w, r)

		// Send initial load
		// TODO: configure
		err := hub.SendWithFilter(func(attrs map[string]interface{}) bool {
			if iteratedClientGUID, ok := attrs["clientGuid"]; ok {
				if iteratedClientGUID == clientGUID.String() {
					return true
				}
			}
			return false
		}, []byte("halloWelt"))
		if err != nil {
			ulog.Errorf("could not send initial load (%s)", err)
		}
	}
}
