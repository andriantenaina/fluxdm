package main

import (
	"fluxdm/internal/core"
	"fluxdm/internal/remote"
)

// RemoteStatus is shown in Options > iPhone / Réseau local.
type RemoteStatus struct {
	Enabled   bool                `json:"enabled"`
	Running   bool                `json:"running"`
	Port      int                 `json:"port"`
	Addresses []remote.Address    `json:"addresses"`
	Devices   []core.RemoteDevice `json:"devices"`
}

// RemoteStatus returns the FluxDM Remote state.
func (a *API) RemoteStatus() RemoteStatus {
	s := a.mgr.Settings()
	devs := make([]core.RemoteDevice, 0, len(s.Remote.Devices))
	for _, d := range s.Remote.Devices {
		d.TokenHash = ""
		devs = append(devs, d)
	}
	return RemoteStatus{Enabled: s.Remote.Enabled, Running: a.ui.svc.remote.Running(), Port: s.Remote.Port,
		Addresses: a.ui.svc.remote.Addresses(), Devices: devs}
}

// RemoteEnable turns FluxDM Remote on or off (and changes the port).
func (a *API) RemoteEnable(on bool, port int) (RemoteStatus, error) {
	s := a.mgr.Settings()
	s.Remote.Enabled = on
	if port > 0 && port < 65536 {
		s.Remote.Port = port
	}
	if err := a.mgr.SetSettings(s); err != nil {
		return a.RemoteStatus(), err
	}
	err := a.ui.svc.remote.Apply()
	return a.RemoteStatus(), err
}

// RemotePairing is a one-time code and the QR codes to scan with the iPhone.
type RemotePairing struct {
	Code      string           `json:"code"`
	Addresses []remote.Address `json:"addresses"`
}

// RemotePair creates a pairing code (valid 10 minutes).
func (a *API) RemotePair() RemotePairing {
	code, addrs := a.ui.svc.remote.NewPairing()
	return RemotePairing{Code: code, Addresses: addrs}
}

// RemoteRevoke forgets a paired device.
func (a *API) RemoteRevoke(id string) error { return a.ui.svc.remote.Revoke(id) }
