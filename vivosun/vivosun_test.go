package vivosun

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// decrypt rebuilds key and IV from Request-Time and Request-Code, as
// Vivosun's server does.
func decrypt(t *testing.T, ts, code string, body []byte) []byte {
	t.Helper()
	var f struct{ Content string }
	if err := json.Unmarshal(body, &f); err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(code, "-", 6)
	if len(parts) != 6 || parts[0] != "AC5" {
		t.Fatalf("Request-Code %q", code)
	}
	var ks, ke, is, ie int
	fmt.Sscan(parts[1], &ks)
	fmt.Sscan(parts[2], &ke)
	fmt.Sscan(parts[3], &is)
	fmt.Sscan(parts[4], &ie)
	salt := parts[5]
	sum := md5.Sum([]byte(ts))
	key := []byte(hex.EncodeToString(sum[:])[ks:ke])
	iv := []byte(salt[is:ie])
	ct, _ := hex.DecodeString(f.Content)
	blk, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(blk, iv).CryptBlocks(out, ct)
	return out[:len(out)-int(out[len(out)-1])]
}

func TestEncrypt_RoundTrip(t *testing.T) {
	t.Parallel()
	for i := range 200 {
		plain := fmt.Appendf(nil, `{"deviceId":"abc","n":%d}`, i)
		ts, code, body := encrypt(plain, 1762560000000+int64(i))
		if got := decrypt(t, ts, code, body); string(got) != string(plain) {
			t.Fatalf("round trip %d: %q", i, got)
		}
		if salt := strings.SplitN(code, "-", 6)[5]; len(salt) < 16 || len(salt) > 99 {
			t.Fatalf("salt length %d", len(salt))
		}
	}
}

func TestHide_AccountDetails(t *testing.T) {
	t.Parallel()
	in := map[string]any{"email": "a@b.c", "userId": "u1", "deviceId": "1234567890", "name": "VCure",
		"setting": map[string]any{"jf": map[string]any{"devPass": "x", "devUser": "y"}, "wifi_name": "home"},
		"note":    "Me@Example.com", "accessToken": "t", "hwId": "AA:BB", "list": []any{"me@example.com"}}
	b, _ := json.Marshal(Hide(in, "ME@example.com"))
	s := string(b)
	for _, bad := range []string{"a@b.c", "u1", "1234567890", `"x"`, `"y"`, "home", "xample.com", `"t"`, "AA:BB"} {
		if strings.Contains(s, bad) {
			t.Fatalf("%s left in %s", bad, s)
		}
	}
	if !strings.Contains(s, "VCure") || !strings.Contains(s, "123456...") {
		t.Fatal(s)
	}

	b, _ = json.Marshal(Hide(map[string]any{"clientId": "vivosun-VSCTLY18-a1b2c3d4e5"}, ""))
	if s = string(b); !strings.Contains(s, `"modelToken":"VSCTLY18"`) || strings.Contains(s, "a1b2c3") {
		t.Fatalf("model token kept, client id hidden: %s", s)
	}
}

// fakeVivosun answers sign-in, the device list and readings.
type fakeVivosun struct {
	t        *testing.T
	refuse   string // message for success:false on every call but sign-in
	status   int    // HTTP status for every call but sign-in (0 = 200)
	readings []map[string]any
	asked    map[string]any // the last decrypted readings request
}

func (f *fakeVivosun) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.URL.Path != loginPath {
		if r.Header.Get("login-token") != "LT" || r.Header.Get("access-token") != "AT" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		if f.refuse != "" {
			fmt.Fprintf(w, `{"success":false,"code":10001,"message":%q}`, f.refuse)
			return
		}
	}
	switch r.URL.Path {
	case loginPath:
		var in map[string]string
		_ = json.Unmarshal(body, &in)
		if in["email"] != "me@example.com" || in["password"] != "pw" || in["spAppId"] != appID {
			fmt.Fprint(w, `{"success":false,"code":20001,"message":"Wrong account or password"}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"accessToken":"AT","loginToken":"LT","userId":"u"}}`)
	case devicesPath:
		fmt.Fprint(w, `{"success":true,"data":{"deviceGroup":{"STORE":[{"deviceId":"1730901","name":"VCure C80","scene":null},{"name":"no id"}],
			"CONTROLLER":[{"deviceId":"99","name":"","scene":{"sceneId":285713}}]}}}`)
	case readingPath:
		plain := decrypt(f.t, r.Header.Get("Request-Time"), r.Header.Get("Request-Code"), body)
		f.asked = map[string]any{}
		_ = json.Unmarshal(plain, &f.asked)
		b, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"iotDataLogList": f.readings}})
		w.Write(b)
	default:
		http.NotFound(w, r)
	}
}

func TestClient_SignInDevicesReadings(t *testing.T) {
	t.Parallel()
	fake := &fakeVivosun{t: t, readings: []map[string]any{
		{"time": 1794105540, "pTemp": 2256, "pHumi": 5737, "bTemp": -6666, "ctrlb.door": 0},
		{"time": 1794105480, "pTemp": 2250},
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	ctx := context.Background()

	if _, err := c.Login(ctx, "me@example.com", "wrong"); !errors.Is(err, ErrAuth) {
		t.Fatalf("wrong password: %v", err)
	}
	tk, err := c.Login(ctx, "me@example.com", "pw")
	if err != nil || tk != (Tokens{Access: "AT", Login: "LT"}) {
		t.Fatalf("sign-in: %+v %v", tk, err)
	}
	devs, err := c.Devices(ctx, tk)
	if err != nil {
		t.Fatal(err)
	}
	want := []Device{{ID: "99", Name: "Vivosun", SceneID: 285713, HasScene: true}, {ID: "1730901", Name: "VCure C80"}}
	if fmt.Sprint(devs) != fmt.Sprint(want) {
		t.Fatalf("devices %+v", devs)
	}
	from := time.Unix(1794105000, 0)
	rows, err := c.Readings(ctx, tk, devs[1], from, from.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if fake.asked["deviceId"] != "1730901" || fake.asked["startTime"] != float64(1794105000) || fake.asked["timeLevel"] != "ONE_MINUTE" {
		t.Fatalf("asked %v", fake.asked)
	}
	if len(rows) != 2 || !rows[0].At.Before(rows[1].At) {
		t.Fatalf("rows not oldest first: %+v", rows)
	}
	last := rows[1].Values
	if last["pTemp"] != 22.56 || last["pHumi"] != 57.37 || last["ctrlb.door"] != 0 {
		t.Fatalf("values %v", last)
	}
	if _, ok := last["bTemp"]; ok {
		t.Fatal("-6666 (no sensor) kept")
	}
}

func TestClient_Refusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fake *fakeVivosun
		auth bool
	}{
		{&fakeVivosun{status: http.StatusForbidden}, true},
		{&fakeVivosun{refuse: "Token expired, please login again"}, true},
		{&fakeVivosun{refuse: "System busy"}, false},
	}
	for _, tc := range cases {
		tc.fake.t = t
		srv := httptest.NewServer(tc.fake)
		_, err := (&Client{BaseURL: srv.URL}).Devices(context.Background(), Tokens{Access: "AT", Login: "LT"})
		srv.Close()
		if err == nil || errors.Is(err, ErrAuth) != tc.auth {
			t.Fatalf("%+v: %v (auth %v)", tc.fake, err, errors.Is(err, ErrAuth))
		}
	}
	_, err := (&Client{BaseURL: "http://127.0.0.1:1"}).Devices(context.Background(), Tokens{Access: "AT", Login: "LT"})
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("unreachable: %v", err)
	}
}

func TestOffers_VCureAndOthers(t *testing.T) {
	t.Parallel()
	// Row values are already divided by 100: cure.comp 0.01 was sent as 1.
	cure := map[string]float64{"pTemp": 22.5, "pHumi": 57, "pVpd": 1.1, "bTemp": 22, "bHumi": 60, "bVpd": 1,
		"coreTemp": 38.9, "evapT": 22, "rssi": -76, "cure.comp": 0.01, "cure.fanCir": 1, "ctrlb.temp": 18}
	var got []string
	for _, o := range Offers("", cure) {
		got = append(got, fmt.Sprintf("%s|%s|%s|%v|%v|%v", o.Type, o.Suffix, o.Unit, o.Value, o.Home, o.More))
	}
	want := []string{"VS.pTemp|temp|°C|22.5|true|false", "VS.pHumi|humidity|%|57|true|false", "VS.pVpd|VPD|kPa|1.1|true|false",
		"VS.bTemp.out|room temp|°C|22|false|false", "VS.bHumi.out|room humidity|%|60|false|false", "VS.bVpd.out|room VPD|kPa|1|false|false",
		"VS.coreTemp|electronics temp|°C|38.9|false|false", "VS.evapT|cooling coil temp|°C|22|false|false",
		"VS.cure.comp|compressor|on/off|1|false|true", "VS.cure.fanCir|circulation fan|%|100|false|true"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("VCure offers:\n%s", strings.Join(got, "\n"))
	}

	ctl := map[string]float64{"inTemp": 25, "inHumi": 50, "outTemp": 20, "coreTemp": 40, "rssi": -60, "light.lv": 0.75}
	got = nil
	for _, o := range Offers("VSCTLE42A", ctl) {
		got = append(got, o.Type+"|"+o.Suffix)
	}
	if strings.Join(got, ",") != "VS.inTemp|inside temp,VS.inHumi|inside humidity,VS.outTemp.out|outside temp,VS.light.lv|light" {
		t.Fatalf("controller offers %v", got)
	}

	for in, want := range map[string]string{"VS.bTemp.out": "bTemp", "VS.pTemp": "pTemp", "ACI.tempF": "", "VS.cure.comp": "cure.comp"} {
		if KeyOf(in) != want {
			t.Fatalf("KeyOf(%s) = %q", in, KeyOf(in))
		}
	}
}

func TestOffers_VGrowAndVSeed(t *testing.T) {
	t.Parallel()
	grow := map[string]float64{"bTemp": 22.61, "pTemp": 22.56, "pCo2": 8.2, "waterT": 24.24, "waterLv": 55.5,
		"light.lv": 0, "light.spec": 1, "usb1.lv": 1, "coreTemp": 40}
	got := map[string]Offer{}
	var order []string
	for _, o := range Offers("VSCTLY18", grow) {
		got[o.Key] = o
		order = append(order, o.Key)
	}
	if strings.Join(order, ",") != "bTemp,pTemp,pCo2,waterT,waterLv,light.lv,light.spec,usb1.lv" {
		t.Fatalf("VGrow order %v", order)
	}
	if o := got["pCo2"]; o.Value != 820 || o.Unit != "ppm" || !o.Home || o.Suffix != "CO₂" {
		t.Fatalf("CO2 %+v", o)
	}
	if o := got["waterLv"]; o.Value != 55.5 || o.Unit != "%" || o.Home {
		t.Fatalf("water level %+v", o)
	}
	if o := got["light.spec"]; o.Value != 100 || !o.More || o.Note != "sm_vs_note_spec" {
		t.Fatalf("spectrum %+v", o)
	}

	seed := map[string]float64{"pTemp": 24, "pHumi": 78, "rTemp": 23.5}
	var names []string
	for _, o := range Offers("VSSBS40", seed) {
		names = append(names, o.Suffix+"|"+o.Note)
	}
	if strings.Join(names, ",") != "dome temp|sm_vs_note_dome,dome humidity|sm_vs_note_dome,root temp|sm_vs_note_root" {
		t.Fatalf("VSeed offers %v", names)
	}
}

func TestValue_Scales(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		model, key string
		in, want   float64
	}{
		{"", "pTemp", 22.56, 22.56},
		{"VSCTLY18", "pCo2", 8.2, 820},      // sent as plain ppm (820)
		{"VSCTLY18", "pCo2", 820, 820},      // sent x100 (82000)
		{"VSCTLY18", "waterLv", 55.5, 55.5}, // x100 on the VGrow
		{"VSHUMH05", "waterLv", 750, 75},    // x1000 on humidifiers
		{"", "usb2.lv", 1, 100},
		{"", "cure.comp", 0.01, 1},
	} {
		if got := Value(c.model, c.key, c.in); got != c.want {
			t.Errorf("Value(%s, %s, %v) = %v, want %v", c.model, c.key, c.in, got, c.want)
		}
	}
	if !IsEquipmentType("VS.dFan.lv") || IsEquipmentType("VS.pTemp") || IsEquipmentType("ACIP.1") {
		t.Fatal("IsEquipmentType")
	}
	for in, want := range map[string]string{"vivosun-VSCTLY18-abc123": "VSCTLY18", "vivosun-vshumh05-x": "VSHUMH05", "vivosun-abc": "", "": ""} {
		if got := modelOf(in); got != want {
			t.Errorf("modelOf(%q) = %q", in, got)
		}
	}
}
