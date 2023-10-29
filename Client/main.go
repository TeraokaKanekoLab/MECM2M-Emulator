package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/m2mapp"
	"mecm2m-Emulator/pkg/psnode"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

var (
	command    *string
	area_num   *int
	target     *string
	vnode_type *string
	ad         string
)

func main() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		log.Fatal(err)
	}

	var data any
	var url string

	command = flag.String("command", "no", "M2M APIを選択")
	area_num = flag.Int("area_num", 1, "指定するエリア範囲を選択")
	target = flag.String("target", "own", "自MEC or 他MEC")

	vnode_type = flag.String("vnode_type", "VSNode", "VNodeの種類")

	flag.Parse()

	data, url = switchM2MAPI(*command)

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

	body, err1 := io.ReadAll(response.Body)
	if err != nil {
		panic(err1)
	}

	switch *command {
	case "area":
		results := &m2mapp.ResolveAreaOutput{}
		if err = json.Unmarshal(body, results); err != nil {
			fmt.Println("Error unmarshaling: ", err)
		}
		path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/Client/ad.txt"
		file, _ := os.Create(path)
		defer file.Close()

		fmt.Fprintf(file, "%s", results.AD)

		//byte_data, _ := json.Marshal(results.AD)
		//file.Write(byte_data)
	case "node":
		results := &m2mapp.ResolveNodeOutput{}
		if err = json.Unmarshal(body, results); err != nil {
			fmt.Println("Error unmarshaling: ", err)
		}
		path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/Client/node.csv"
		file, _ := os.Create(path)
		defer file.Close()

		for _, i := range results.VNode {
			fmt.Fprintf(file, "%v,%v,%v\n", i.VNodeID, i.VNodeSocketAddress, i.VMNodeRSocketAddress)
		}
	}

	// fmt.Println("Server Response:", results)

	elapsedTime := time.Since(start)
	durationInNanoseconds := float64(elapsedTime.Nanoseconds())
	durationInMilliSeconds := durationInNanoseconds / 1e6
	fmt.Printf("%.3f\n", durationInMilliSeconds)
}

func switchM2MAPI(command string) (data any, url string) {
	switch command {
	case "area":
		var nelat, nelon, swlat, swlon float64
		var file_name string
		if *target == "own" {
			if *area_num == 1 {
				file_name = "area/area_1_1_own.csv"
			} else if *area_num == 5 {
				file_name = "area/area_5_5_own.csv"
			} else if *area_num == 10 {
				file_name = "area/area_10_10_own.csv"
			}
		} else if *target == "other" {
			if *area_num == 1 {
				file_name = "area/area_1_1_other.csv"
			} else if *area_num == 5 {
				file_name = "area/area_5_5_other.csv"
			} else if *area_num == 10 {
				file_name = "area/area_10_10_other.csv"
			}
		}
		file, _ := os.Open(file_name)
		defer file.Close()

		reader := csv.NewReader(file)
		rows, _ := reader.ReadAll()
		randomIndex := rand.Intn(len(rows))
		nelat, _ = strconv.ParseFloat(rows[randomIndex][0], 64)
		nelon, _ = strconv.ParseFloat(rows[randomIndex][1], 64)
		swlat, _ = strconv.ParseFloat(rows[randomIndex][2], 64)
		swlon, _ = strconv.ParseFloat(rows[randomIndex][3], 64)
		data = m2mapp.ResolveAreaInput{
			NE: m2mapp.SquarePoint{Lat: nelat, Lon: nelon},
			SW: m2mapp.SquarePoint{Lat: swlat, Lon: swlon},
		}
		url = "http://localhost:8080/m2mapi/area"
	case "node":
		file, _ := os.Open("ad.txt")
		defer file.Close()

		dat, _ := io.ReadAll(file)
		ad = string(dat)
		fmt.Println(ad)
		data = m2mapp.ResolveNodeInput{
			AD:         ad,
			Capability: []string{"Temperature", "Humidity", "WindSpeed"},
			NodeType:   *vnode_type,
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
		file, _ := os.Open("node.csv")
		defer file.Close()

		reader := csv.NewReader(file)
		rows, _ := reader.ReadAll()

		randomIndex := rand.Intn(len(rows))
		vnode_id := rows[randomIndex][0]
		socket_address := rows[randomIndex][1]
		data = m2mapp.ResolveDataByNodeInput{
			VNodeID:       vnode_id,
			Capability:    []string{"Temperature", "Humidity", "WindSpeed"},
			SocketAddress: socket_address,
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
