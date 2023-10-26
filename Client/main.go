package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/m2mapp"
	"mecm2m-Emulator/pkg/psnode"
	"net/http"
	"os"
	"time"
)

func main() {
	var ad string
	var data any
	var url string
	args := os.Args

	if len(args) > 2 {
		ad = args[2]
	}
	data, url = switchM2MAPI(args[1], ad)

	client_data, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshalling data: ", err)
		return
	}

	// 実行時間の計測
	start := time.Now()

	response, err := http.Post(url, "application/json", bytes.NewBuffer(client_data))
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}

	results := formatBody(args[1], body)

	fmt.Println("Server Response:", results)

	elapsedTime := time.Since(start)
	fmt.Println("execution time: ", elapsedTime)
}

func switchM2MAPI(command, ad string) (data any, url string) {
	switch command {
	case "area":
		data = m2mapp.ResolveAreaInput{
			NE: m2mapp.SquarePoint{Lat: 35.531, Lon: 139.531},
			SW: m2mapp.SquarePoint{Lat: 35.530, Lon: 139.530},
		}
		url = "http://localhost:8080/m2mapi/area"
	case "node":
		data = m2mapp.ResolveNodeInput{
			AD:         ad,
			Capability: []string{"Temperature", "Humidity", "WindSpeed"},
			NodeType:   "VSNode",
		}
		url = "http://localhost:8080/m2mapi/node"
	case "past_node":
		data = m2mapp.ResolveDataByNodeInput{
			VNodeID:       "9223372036854775808",
			Capability:    []string{"Temperature", "Humidity", "WindSpeed"},
			Period:        m2mapp.PeriodInput{Start: "2023-10-21 01:00:00 +0900 JST", End: "2023-10-21 01:30:00 +0900 JST"},
			SocketAddress: "192.168.1.1:11000",
		}
		url = "http://localhost:8080/m2mapi/data/past/node"
	case "current_node":
		data = m2mapp.ResolveDataByNodeInput{
			VNodeID:       "9223372036854775808",
			Capability:    []string{"Temperature", "Humidity", "WindSpeed"},
			SocketAddress: "192.168.1.1:11000",
		}
		url = "http://localhost:8080/m2mapi/data/current/node"
	case "condition_node":
		data = m2mapp.ResolveDataByNodeInput{
			VNodeID:       "9223372036854775808",
			Capability:    []string{"Temperature", "Humidity", "WindSpeed"},
			Condition:     m2mapp.ConditionInput{Limit: m2mapp.Range{LowerLimit: 30, UpperLimit: 39}, Timeout: 10 * time.Second},
			SocketAddress: "192.168.1.1:11000",
		}
		url = "http://localhost:8080/m2mapi/data/condition/node"
	case "past_area":
		data = m2mapp.ResolveDataByAreaInput{
			AD:         ad,
			Capability: []string{"Temperature", "Humidity", "WindSpeed"},
			Period:     m2mapp.PeriodInput{Start: "2023-10-21 01:00:00 +0900 JST", End: "2023-10-21 01:30:00 +0900 JST"},
			NodeType:   "VSNode",
		}
		url = "http://localhost:8080/m2mapi/data/past/area"
	case "current_area":
		data = m2mapp.ResolveDataByAreaInput{
			AD:         ad,
			Capability: []string{"Temperature", "Humidity", "WindSpeed"},
			NodeType:   "VSNode",
		}
		url = "http://localhost:8080/m2mapi/data/current/area"
	case "condition_area":
		data = m2mapp.ResolveDataByAreaInput{
			AD:         ad,
			Capability: []string{"Temperature", "Humidity", "WindSpeed"},
			Condition:  m2mapp.ConditionInput{Limit: m2mapp.Range{LowerLimit: 30, UpperLimit: 40}, Timeout: 10 * time.Second},
			NodeType:   "VSNode",
		}
		url = "http://localhost:8080/m2mapi/data/condition/area"
	case "actuate":
		data = m2mapp.ActuateInput{
			VNodeID:       "9223372036854775808",
			Capability:    "Accel",
			Action:        "On",
			Parameter:     10.1,
			SocketAddress: "192.168.1.1:11000",
		}
		url = "http://localhost:8080/m2mapi/actuate"
	case "extend_ad":
		data = m2mapi.ExtendAD{
			AD: "c000121688",
		}
		url = "http://localhost:8080/m2mapi/area/extend"
	case "time":
		data = psnode.TimeSync{
			PNodeID:     "2305843009213693952",
			CurrentTime: time.Now(),
		}
		url = "http://localhost:21000/time"
	default:
		fmt.Println("There is no args")
		log.Fatal()
	}
	return data, url
}

func formatBody(command string, body []byte) string {
	var results string
	switch command {
	case "area":
		/*
			format := m2mapp.ResolveAreaOutput{}
			if err := json.Unmarshal(body, &format); err != nil {
				fmt.Println("Error unmarshaling: ", err)
				return results
			}
			format.Descriptor = m2mapi.AreaDescriptor{}
			results_byte, err := json.Marshal(format)
			if err != nil {
				fmt.Println("Error marshaling: ", err)
				return results
			}
		*/
		return string(body)
	case "node":
		return string(body)
	case "past_node":
		return string(body)
	case "current_node":
		return string(body)
	case "condition_node":
		return string(body)
	case "past_area":
		return string(body)
	case "current_area":
		return string(body)
	case "condition_area":
		return string(body)
	case "actuate":
		return string(body)
	case "time":
		return string(body)
	default:
		return results
	}
}
