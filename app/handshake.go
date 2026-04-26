package app

import gomodule "github.com/hashicorp/go-plugin"

// Handshake is the shared handshake config. Both MASS and all apps
// must use this exact config. The magic cookie prevents accidental
// execution of the app binary outside of MASS.
var Handshake = gomodule.HandshakeConfig{
	ProtocolVersion:  2,
	MagicCookieKey:   "MASS_APP",
	MagicCookieValue: "mass-v1",
}

// AppName is the key used in the go-plugin PluginSet map.
const AppName = "app"

// SDKVersion is the string form of Handshake.ProtocolVersion.
// App repos reference this when generating their app.yml metadata.
const SDKVersion = "2"
