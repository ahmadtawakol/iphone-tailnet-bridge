package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/tailscale"
)

type StatusDocument struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
	Devices   []Status  `json:"devices"`
}

type Service struct {
	Paths     config.Paths
	Tailscale *tailscale.Client

	mutex    sync.Mutex
	statuses map[string]Status
}

func (service *Service) Run(ctx context.Context, profiles []config.Profile) error {
	if service.statuses == nil {
		service.statuses = map[string]Status{}
	}
	var enabled []config.Profile
	for _, profile := range profiles {
		if profile.Enabled {
			enabled = append(enabled, profile)
		} else {
			service.update(Status{ID: profile.ID, Name: profile.Name, State: StateOff, Message: "Bridge disabled", UpdatedAt: time.Now().UTC()})
		}
	}
	if len(enabled) == 0 {
		<-ctx.Done()
		return nil
	}
	if len(enabled) > 1 {
		return fmt.Errorf("only one device bridge can be active at a time because CoreDevice reuses local ports; disable all but one profile")
	}
	controller := Controller{Profile: enabled[0], Tailscale: service.Tailscale, Update: service.update}
	return controller.Run(ctx)
}

func (service *Service) update(status Status) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.statuses[status.ID] = status
	document := StatusDocument{Version: 1, UpdatedAt: time.Now().UTC()}
	for _, current := range service.statuses {
		document.Devices = append(document.Devices, current)
	}
	_ = config.WriteJSONPrivate(service.Paths.Status, document)
}
