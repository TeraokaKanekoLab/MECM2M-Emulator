package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mecm2m-Emulator/pkg/m2mapp"

	"github.com/joho/godotenv"
)

func main() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		log.Fatal(err)
	}

	app_type := flag.String("type", "", "アプリケーションのタイプ: node or area")
	nelat := flag.Float64("nelat", 0.0, "矩形範囲の北東隅の緯度")
	nelon := flag.Float64("nelon", 0.0, "矩形範囲の北東隅の軽度")
	swlat := flag.Float64("swlat", 0.0, "矩形範囲の南西隅の緯度")
	swlon := flag.Float64("swlon", 0.0, "矩形範囲の南西隅の軽度")
	lower_range := flag.Float64("lower_range", 0.0, "条件の最小値")
	upper_range := flag.Float64("upper_range", 0.0, "条件の最大値")
	timeout_int := flag.Int("timeout", 0, "充足条件のタイムアウト時間")

	flag.Parse()
	timeout := time.Duration(*timeout_int) * time.Second

	if *app_type == "node" {
		// ノード指定型
		// エリア解決
		area_input_data := m2mapp.ResolveAreaInput{
			NE: m2mapp.SquarePoint{Lat: *nelat, Lon: *nelon},
			SW: m2mapp.SquarePoint{Lat: *swlat, Lon: *swlon},
		}
		area_url := "http://localhost:8080/m2mapi/area"

		area_client_data, err := json.Marshal(area_input_data)
		if err != nil {
			fmt.Println("Error marshal data: ", err)
			return
		}

		area_response, err := http.Post(area_url, "application/json", bytes.NewBuffer(area_client_data))
		if err != nil {
			fmt.Println("Error making request: ", err)
			return
		}
		defer area_response.Body.Close()

		area_body, err := io.ReadAll(area_response.Body)
		if err != nil {
			fmt.Println("Error read all: ", err)
			return
		}

		var area_result m2mapp.ResolveAreaOutput
		if err = json.Unmarshal(area_body, &area_result); err != nil {
			fmt.Println("Error unmarshaling: ", err)
			return
		}
		ad := area_result.AD
		fmt.Println("[Resolve Area] Done")

		// **************************************************
		// ノード解決
		node_input_data := m2mapp.ResolveNodeInput{
			AD:         ad,
			Capability: []string{"Temperature", "Humidity", "WindSpeed"},
			NodeType:   "VSNode",
		}
		node_url := "http://localhost:8080/m2mapi/node"

		node_client_data, err := json.Marshal(node_input_data)
		if err != nil {
			fmt.Println("Error marshal data: ", err)
			return
		}

		node_response, err := http.Post(node_url, "application/json", bytes.NewBuffer(node_client_data))
		if err != nil {
			fmt.Println("Error making request: ", err)
			return
		}
		defer node_response.Body.Close()

		node_body, err := io.ReadAll(node_response.Body)
		if err != nil {
			fmt.Println("Error read all: ", err)
			return
		}

		var node_result m2mapp.ResolveNodeOutput
		if err = json.Unmarshal(node_body, &node_result); err != nil {
			fmt.Println("Error unmarshaling: ", err)
			return
		}
		randomIndex := rand.Intn(len(node_result.VNode))
		vnode_id := node_result.VNode[randomIndex].VNodeID
		socket_address := node_result.VNode[randomIndex].VNodeSocketAddress
		fmt.Printf("[Resolve Node] Done (VNodeID: %s)\n", vnode_id)

		// **************************************************
		// 定期的なノード指定型現在データ取得
		t := time.NewTicker(time.Duration(5) * time.Second)
		// シグナル受信用チャネル
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		defer signal.Stop(sig)

		var wg sync.WaitGroup
		wg.Add(2)

		controlChannel := make(chan string)

		go func() {
			resolveCurrentNodeLoop(t, vnode_id, socket_address, sig, controlChannel)
			wg.Done()
		}()

		// **************************************************
		// 同時にノード指定型充足条件データ取得を投げる

		go func() {
			resolveConditionNodeLoop(vnode_id, socket_address, *lower_range, *upper_range, timeout, controlChannel)
			wg.Done()
		}()

		wg.Wait()
	} else if *app_type == "area" {
		// エリア指定型
	} else {
		fmt.Println("There is no app type")
	}
}

func resolveCurrentNodeLoop(t *time.Ticker, vnode_id string, socket_address string, sig chan os.Signal, controlChannel chan string) {
	for {
		select {
		case <-t.C:
			// ノード指定型データ取得を投げる
			current_node_input_data := m2mapp.ResolveDataByNodeInput{
				VNodeID:       vnode_id,
				Capability:    []string{"Temperature", "Humidity", "WindSpeed"},
				SocketAddress: socket_address,
			}
			current_node_url := "http://localhost:8080/m2mapi/data/current/node"

			current_node_client_data, err := json.Marshal(current_node_input_data)
			if err != nil {
				fmt.Println("Error marshal data: ", err)
				return
			}

			current_node_response, err := http.Post(current_node_url, "application/json", bytes.NewBuffer(current_node_client_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return
			}
			defer current_node_response.Body.Close()

			current_node_body, err := io.ReadAll(current_node_response.Body)
			if err != nil {
				fmt.Println("Error read all: ", err)
				return
			}

			var current_node_result m2mapp.ResolveDataByNodeOutput
			if err = json.Unmarshal(current_node_body, &current_node_result); err != nil {
				fmt.Println("Error unmarshaling: ", err)
				return
			}
			vnode_id := current_node_result.VNodeID
			capability := current_node_result.Values[0].Capability
			timestamp := current_node_result.Values[0].Time
			value := current_node_result.Values[0].Value
			fmt.Println("[Resolve Current Node] Done")
			fmt.Println("*********************************************")
			fmt.Printf("* VNodeID:    %s *\n", vnode_id)
			fmt.Printf("* Capability: %s *\n", capability)
			fmt.Printf("* Time:       %s *\n", timestamp)
			fmt.Printf("* Value:      %f *\n", value)
			fmt.Println("*********************************************")
			fmt.Printf("\n")
		// シグナルを受信した場合
		case s := <-sig:
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				fmt.Println("Stop!")
				return
			}
		case <-controlChannel:
			return
		}
	}
}

func resolveConditionNodeLoop(vnode_id, socket_address string, lower_range, upper_range float64, timeout time.Duration, controlChannel chan string) {
	for {
		condition_node_input_data := m2mapp.ResolveDataByNodeInput{
			VNodeID:       vnode_id,
			Capability:    []string{"Temperature", "Humidity", "WindSpeed"},
			Condition:     m2mapp.ConditionInput{Limit: m2mapp.Range{LowerLimit: lower_range, UpperLimit: upper_range}, Timeout: timeout},
			SocketAddress: socket_address,
		}
		condition_node_url := "http://localhost:8080/m2mapi/data/condition/node"

		condition_node_client_data, err := json.Marshal(condition_node_input_data)
		if err != nil {
			fmt.Println("Error marshal data: ", err)
			return
		}

		condition_node_response, err := http.Post(condition_node_url, "application/json", bytes.NewBuffer(condition_node_client_data))
		if err != nil {
			fmt.Println("Error making request: ", err)
			return
		}
		defer condition_node_response.Body.Close()

		condition_node_body, err := io.ReadAll(condition_node_response.Body)
		if err != nil {
			fmt.Println("Error read all: ", err)
			return
		}

		var condition_node_result m2mapp.ResolveDataByNodeOutput
		if err = json.Unmarshal(condition_node_body, &condition_node_result); err != nil {
			fmt.Println("Error unmarshaling: ", err)
			return
		}

		result_vnode_id := condition_node_result.VNodeID
		if result_vnode_id == "Timeout" {
			fmt.Println("[Resolve Condition Node] Failed (Session Timeout)")
		} else {
			capability := condition_node_result.Values[0].Capability
			timestamp := condition_node_result.Values[0].Time
			value := condition_node_result.Values[0].Value
			fmt.Println("[Resolve Condition Node] Done")
			fmt.Println("*********************************************")
			fmt.Printf("* VNodeID:    %s *\n", result_vnode_id)
			fmt.Printf("* Capability: %s *\n", capability)
			fmt.Printf("* Time:       %s *\n", timestamp)
			fmt.Printf("* Value:      %f *\n", value)
			fmt.Println("*********************************************")
		}

		//controlChannel <- "done"
	}
}
