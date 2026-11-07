package lanhelper

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Set only by the dedicated reviewed build. No environment, request or manifest
// field can substitute a key. Empty/default builds cannot authorize LAN setup.
var bundleVerifierPublicKeyHex string

func VerifyConfiguredAssetBundle(data, signature []byte) (AssetBundle, error) {
	return VerifyAssetBundle(data, signature, bundleVerifierPublicKeyHex)
}

const bundleSigningDomain = "orbwin-lan-asset-bundle-v1\n"

type BundleAsset struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   uint64 `json:"size"`
}
type AssetBundle struct {
	Version int           `json:"version"`
	Assets  []BundleAsset `json:"assets"`
}

// VerifyAssetBundle is a pure bounded verifier. A verified manifest is still not
// native asset identity: the helper must hold and hash each actual fixed file.
func VerifyAssetBundle(data, signature []byte, publicKeyHex string) (AssetBundle, error) {
	var empty AssetBundle
	key, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(key) != ed25519.PublicKeySize || hex.EncodeToString(key) != publicKeyHex || bytes.Equal(key, make([]byte, ed25519.PublicKeySize)) {
		return empty, errors.New("LAN bundle verifier public key is unconfigured or malformed")
	}
	if len(data) == 0 || len(data) > 4096 || len(signature) != 88 {
		return empty, errors.New("LAN detached bundle exceeds fixed bounds")
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(string(signature))
	if err != nil || len(sig) != ed25519.SignatureSize || base64.StdEncoding.EncodeToString(sig) != string(signature) {
		return empty, errors.New("LAN detached signature encoding differs")
	}
	if !ed25519.Verify(ed25519.PublicKey(key), append([]byte(bundleSigningDomain), data...), sig) {
		return empty, errors.New("LAN asset bundle signature is not trusted")
	}
	var bundle AssetBundle
	if ValidateClosedJSON(data, AssetBundle{}) != nil || json.Unmarshal(data, &bundle) != nil || bundle.Version != 1 || len(bundle.Assets) != 3 {
		return empty, errors.New("LAN bundle closed schema differs")
	}
	canonical, _ := json.Marshal(bundle)
	if !bytes.Equal(data, canonical) {
		return empty, errors.New("LAN bundle must use exact canonical JSON bytes")
	}
	roles := []string{"manager", "helper", "worker"}
	paths := []string{"orbwin-session.exe", "lan/orbwin-lan-helper.exe", "lan/orbwin-lan-worker.exe"}
	limits := []uint64{256 << 20, 64 << 20, 64 << 20}
	for index, asset := range bundle.Assets {
		if asset.Role != roles[index] || asset.Path != paths[index] || !ValidSHA(asset.SHA256) || asset.Size == 0 || asset.Size > limits[index] {
			return empty, errors.New("LAN bundle fixed role/path/digest/size differs")
		}
	}
	return bundle, nil
}
