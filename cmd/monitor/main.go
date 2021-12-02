package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MOXA-ISD/edge-thingspro-agent/internal/util"
	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// TagfResponse ...
type TagfResponse struct {
	PrvdName      string      `json:"prvdName"`
	SrcName       string      `json:"srcName"`
	TagName       string      `json:"tagName"`
	Value         interface{} `json:"dataValue"`
	Ts            uint64      `json:"ts"`
	Type          string      `json:"dataType"`
	Description   string      `json:"description,omitempty"`
	EventSeverity string      `json:"eventSeverity,omitempty"`
	EventUser     string      `json:"eventUser,omitempty"`
}

const (
	Uint8     = "uint8"
	Uint16    = "uint16"
	Uint32    = "uint32"
	Uint64    = "uint64"
	Int8      = "int8"
	Int16     = "int16"
	Int32     = "int32"
	Int64     = "int64"
	Float     = "float"
	Double    = "double"
	String    = "string"
	Boolean   = "boolean"
	Bytearray = "bytearray"
	Raw       = "raw"
)

var (
	counterMetrics = map[string]*Counter{}
)

func tag2Number(key string, tag TagfResponse) *Counter {
	// connection metrics
	if strings.Index(tag.PrvdName, "$connection_") == 0 {
		if strings.Index(tag.SrcName, "store") == 0 {
			// device => garyDevice1
			// connection => azuredevice1 // prvdName
			// store => store1 // srcName
			return NewNumber(prometheus.NewDesc(
				key, "", []string{"device", "connection", "store"}, nil,
			))
		}

		if strings.Index(tag.SrcName, "messageGroup") == 0 {
			// device => garyDevice1
			// connection => azuredevice1 // prvdName
			// store => store1 // srcName
			return NewNumber(prometheus.NewDesc(
				key, "", []string{"device", "connection", "messageGroup"}, nil,
			))
		}

		return NewNumber(prometheus.NewDesc(
			key, "", []string{"device", "connection"}, nil,
		))
	}

	// other
	return NewNumber(prometheus.NewDesc(
		key, "", []string{"device"}, nil,
	))
}

func setTag2Number(origin, key string, tag TagfResponse) {
	// connection metrics
	if strings.Index(tag.PrvdName, "$connection_") == 0 {
		if strings.Index(tag.SrcName, "store") == 0 {
			counterMetrics[key].Set(parseValue(tag), []string{
				origin,
				strings.Replace(tag.PrvdName, "$connection_", "", 1),
				tag.SrcName})
			return
		}

		if strings.Index(tag.SrcName, "messageGroup") == 0 {
			counterMetrics[key].Set(parseValue(tag), []string{
				origin,
				strings.Replace(tag.PrvdName, "$connection_", "", 1),
				tag.SrcName})
			return
		}

		counterMetrics[key].Set(parseValue(tag), []string{
			origin,
			strings.Replace(tag.PrvdName, "$connection_", "", 1)})
		return
	}

	// other
	counterMetrics[key].Set(parseValue(tag), []string{origin})
}

func processUpdate(origin string, payload []byte) {
	var tags []TagfResponse

	if err := jsoniter.Unmarshal(payload, &tags); err != nil {
		logrus.Warnln("failed to unmarshal input payload, err:", err)
		return
	}

	for _, tag := range tags {
		key := toKey(origin, tag)
		if counterMetrics[key] != nil {
			setTag2Number(origin, key, tag)
			continue
		}

		// create a counter pointer
		mCounter := tag2Number(key, tag)

		// save it
		counterMetrics[key] = mCounter
		// register the metric
		prometheus.MustRegister(mCounter)
		// add the first value
		setTag2Number(origin, key, tag)
	}
}

func loginTPE(device *Device) (string, string, error) {
	protocol := "http"
	user := "admin"
	password := "admin@123"
	if device.User != "" {
		user = device.User
	}
	if device.Password != "" {
		password = device.Password
	}
	if device.TLSEnable {
		protocol = "https"
	}

	_, out, err := util.HTTPRequest(http.MethodPost, fmt.Sprintf("%s://%s/api/v1/auth", protocol, device.Addr), fmt.Sprintf("{\"name\":\"%s\",\"password\":\"%s\"}", user, password))
	if err != nil {
		return "", "", fmt.Errorf("failed to login to device:%s, err:%v", device.Addr, err)
	}

	result := gjson.Get(out, "data.token")
	if !result.Exists() {
		return "", "", fmt.Errorf("failed to get token of device:%s, response:%s", device.Addr, out)
	}
	token := result.String()
	_, out, err = util.HTTPRequestV2(http.MethodGet, fmt.Sprintf("%s://%s/api/v1/auth/websocket-token", protocol, device.Addr), "", map[string]string{
		"mx-api-token": result.String(),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to get websocket token of device:%s, err:%v", device.Addr, err)
	}

	result = gjson.Get(out, "data.token")
	if !result.Exists() {
		return "", "", fmt.Errorf("failed to get websocket token of device:%s, response:%s", device.Addr, out)
	}
	return token, result.String(), nil
}

type DeviceInfo struct {
	modelName        string
	serialNumber     string
	thingsproVersion string
	wan              string
}

func InfoToMetrics(info DeviceInfo, origin string) {
	logrus.Infoln(info)

	key := "deviceInfo"
	if counterMetrics[key] == nil {
		mCounter := NewNumber(prometheus.NewDesc(
			key, "", []string{"device", "modelName", "serialNumber", "thingsproVersion", "wan"}, nil,
		))
		counterMetrics[key] = mCounter
		prometheus.MustRegister(mCounter)
	}
	counterMetrics[key].Set(0, []string{origin, info.modelName, info.serialNumber, info.thingsproVersion, info.wan})
}

func GetDeviceInfo(device *Device, token string) DeviceInfo {
	dev := DeviceInfo{}

	protocol := "http"
	if device.TLSEnable {
		protocol = "https"
	}

	_, out, err := util.HTTPRequestV2(http.MethodGet, fmt.Sprintf("%s://%s/api/v1/device/general", protocol, device.Addr), "", map[string]string{
		"mx-api-token": token,
	})
	if err != nil {
		logrus.Warnf("failed to get device/general, err:%v", err)
		return dev
	}
	general := gjson.Get(out, "data")
	dev.modelName = general.Get("modelName").String()
	dev.serialNumber = general.Get("serialNumber").String()
	dev.thingsproVersion = general.Get("thingsproVersion").String()

	_, out, err = util.HTTPRequestV2(http.MethodGet, fmt.Sprintf("%s://%s/api/v1/device/network/wan", protocol, device.Addr), "", map[string]string{
		"mx-api-token": token,
	})
	if err != nil {
		logrus.Warnf("failed to get device/network, err:%v", err)
		return dev
	}

	wan := gjson.Get(out, "data")
	dev.wan = wan.Get("displayName").String()
	return dev
}

func start(device *Device, ctx context.Context) {
	protocol := "ws"
	if device.TLSEnable {
		protocol = "wss"
	}

	for ctx.Err() == nil {

		logrus.Infof("connecting to %s\n", device.Addr)

		apiToken, token, err := loginTPE(device)
		if err != nil {
			logrus.Warnln(err)
			time.Sleep(3 * time.Second)
			continue
		}
		logrus.Infoln("[1] login suceesfully")

		InfoToMetrics(GetDeviceInfo(device, apiToken), device.Name)
		u := fmt.Sprintf("%s://%s/api/v1/http/1?token=%s", protocol, device.Addr, token)
		dialer := websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: nil, InsecureSkipVerify: true}}
		c, _, err := dialer.Dial(u, nil)
		if err != nil {
			logrus.Warnln("dial:", err)
			time.Sleep(3 * time.Second)
			continue
		}
		device.conn = c
		logrus.Infoln("[2] websocket dial suceesfully")

		for ctx.Err() == nil {
			_, message, err := c.ReadMessage()
			if err != nil {
				logrus.Warnln("error:", err)
				break
			}
			logrus.Infof("recv: %s\n", message)
			processUpdate(device.Name, message)
		}

		c.Close()
	}
}

/*
 paremeter:
	* devices addr: 10.123.12.138:80/api/v1/metrics,10.123.13.49:80/api/v1/metrics
 remote -> device: $ADDR request monitor (WS)
 device -> streaming -> remote: update data
 [
    {
        "name": "gary1",
		"tlsEnable": true,
        "addr": "10.123.12.138:8443"
    }
 ]
*/
type Device struct {
	Name      string `json:"name"`
	TLSEnable bool   `json:"tlsEnable"`
	Addr      string `json:"addr"`
	User      string `json:"user"`
	Password  string `json:"password"`
	conn      *websocket.Conn
}

func main() {

	var devices []Device

	if os.Getenv("DEVICES") == "" {
		logrus.Panicln("please input devices' configuration")
	}

	if err := jsoniter.Unmarshal([]byte(os.Getenv("DEVICES")), &devices); err != nil {
		logrus.Panicln("input incorrect devices' configuration, err:", err)
	}

	if len(devices) <= 0 {
		logrus.Panicln("input incorrect devices' configuration, err: empty")
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.TODO())

	for i := range devices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer logrus.Infof("device:%s receiver stopped", devices[i].Name)
			start(&devices[i], ctx)
		}()
		logrus.Infof("device:%s receiver is running", devices[i].Name)
	}

	go func() {
		prometheus.Unregister(prometheus.NewGoCollector())
		http.Handle("/metrics", promhttp.Handler())
		logrus.Fatal(http.ListenAndServe(":8080", nil))
	}()
	logrus.Infoln("metrics exporter is running")

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT)
	signal.Notify(c, syscall.SIGTERM)
	<-c
	for i := range devices {
		if devices[i].conn != nil {
			devices[i].conn.Close()
		}
	}
	cancel()
	wg.Wait()
	logrus.Infoln("all stopped")
}
