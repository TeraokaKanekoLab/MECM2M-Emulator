package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"

	"github.com/joho/godotenv"
)

var port string
var graphdb_url string
var graphdb_url_global string

var global_flag bool

func main() {
	loadEnv()
	port = os.Getenv("M2M_API_PORT")
	graphdb_url = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@localhost:" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
	graphdb_url_global = "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + os.Getenv("CLOUD_SERVER_IP_ADDRESS") + ":" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"

	http.HandleFunc("/m2mapi/point", resolvePoint)
	http.HandleFunc("/m2mapi/node", resolveNode)
	http.HandleFunc("/m2mapi/data/past/node", resolvePastNode)

	log.Printf("Connect to http://%s:%s/ for M2M API", "192.168.1.1", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func resolvePoint(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolvePoint: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolvePoint{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePoint: Error missmatching packet format", http.StatusInternalServerError)
		}

		// GraphDBへの問い合わせ
		results := resolvePointFunction(inputFormat.SW, inputFormat.NE)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolvePoint: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveNode(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// GraphDBへの問い合わせ
		results := resolveNodeFunction(inputFormat.VPointID, inputFormat.CapsInput)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolveNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolvePastNode(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolvePastNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolvePastNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePastNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNodeへリクエスト転送
		results := resolvePastNodeFunction(inputFormat.VNodeID, inputFormat.Capability, inputFormat.SocketAddress, inputFormat.Period)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolvePastNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolvePointFunction(sw, ne m2mapi.SquarePoint) []m2mapi.ResolvePoint {
	// エッジサーバのカバー領域をもとに，検索範囲が少しでもカバー領域から外れていればクラウドサーバのDBへ検索
	// そうでなければ，通常通りローカルのDBへ検索
	min_lat, _ := strconv.ParseFloat(os.Getenv("MIN_LAT"), 64)
	max_lat, _ := strconv.ParseFloat(os.Getenv("MAX_LAT"), 64)
	min_lon, _ := strconv.ParseFloat(os.Getenv("MIN_LON"), 64)
	max_lon, _ := strconv.ParseFloat(os.Getenv("MAX_LON"), 64)

	payload := `{"statements": [{"statement": "MATCH (a:Area)-[:isVirtualizedBy]->(vp:VPoint) WHERE a.NE[0] > ` + strconv.FormatFloat(sw.Lat, 'f', 4, 64) + ` and a.NE[1] > ` + strconv.FormatFloat(sw.Lon, 'f', 4, 64) + ` and a.SW[0] < ` + strconv.FormatFloat(ne.Lat, 'f', 4, 64) + ` and a.SW[1] < ` + strconv.FormatFloat(ne.Lon, 'f', 4, 64) + ` return vp.VPointID, vp.SocketAddress;"}]}`
	var req *http.Request
	if (ne.Lat > max_lat && ne.Lon > max_lon) || (sw.Lat < min_lat && sw.Lon < min_lon) {
		fmt.Println("request to global")
		req, _ = http.NewRequest("POST", graphdb_url_global, bytes.NewBuffer([]byte(payload)))
		global_flag = true
	} else {
		req, _ = http.NewRequest("POST", graphdb_url, bytes.NewBuffer([]byte(payload)))
		global_flag = false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		message.MyError(err, "resolvePointFunction > client.Do()")
	}
	defer resp.Body.Close()

	byteArray, _ := ioutil.ReadAll(resp.Body)
	values := BodyGraphQL(byteArray)

	var row_data interface{}
	results := []m2mapi.ResolvePoint{}
	for _, v1 := range values {
		for k2, v2 := range v1.(map[string]interface{}) {
			if k2 == "data" {
				for _, v3 := range v2.([]interface{}) {
					for k4, v4 := range v3.(map[string]interface{}) {
						if k4 == "row" {
							row_data = v4
							dataArray := row_data.([]interface{})
							result := m2mapi.ResolvePoint{}
							result.VPointID = dataArray[0].(string)
							result.SocketAddress = dataArray[1].(string)
							results = append(results, result)
						}
					}
				}
			}
		}
	}

	return results
}

func resolveNodeFunction(vpoint_id string, caps []string) []m2mapi.ResolveNode {
	// エッジサーバのカバー領域をもとに，検索範囲が少しでもカバー領域から外れていればクラウドサーバのDBへ検索
	// そうでなければ，通常通りローカルのDBへ検索

	var format_vpoint_id string
	format_vpoint_id = "\\\"" + vpoint_id + "\\\""
	var format_capabilities []string
	for _, capability := range caps {
		capability = "\\\"" + capability + "\\\""
		format_capabilities = append(format_capabilities, capability)
	}
	payload := `{"statements": [{"statement": "MATCH (vp:VPoint {VPointID: ` + format_vpoint_id + `})-[:aggregates]->(vn:VNode)-[:isPhysicalizedBy]->(pn:PNode) WHERE pn.Capability IN [` + strings.Join(format_capabilities, ", ") + `] return vn.VNodeID, pn.Capability, vn.SocketAddress;"}]}`
	var req *http.Request
	if global_flag {
		fmt.Println("request to global")
		req, _ = http.NewRequest("POST", graphdb_url_global, bytes.NewBuffer([]byte(payload)))
	} else {
		req, _ = http.NewRequest("POST", graphdb_url, bytes.NewBuffer([]byte(payload)))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		message.MyError(err, "resolvePointFunction > client.Do()")
	}
	defer resp.Body.Close()

	byteArray, _ := ioutil.ReadAll(resp.Body)
	values := BodyGraphQL(byteArray)

	var row_data interface{}
	results := []m2mapi.ResolveNode{}
	for _, v1 := range values {
		for k2, v2 := range v1.(map[string]interface{}) {
			if k2 == "data" {
				for _, v3 := range v2.([]interface{}) {
					for k4, v4 := range v3.(map[string]interface{}) {
						if k4 == "row" {
							row_data = v4
							dataArray := row_data.([]interface{})
							result := m2mapi.ResolveNode{}
							result.VNodeID = dataArray[0].(string)
							result.CapOutput = dataArray[1].(string)
							result.SocketAddress = dataArray[2].(string)
							results = append(results, result)
						}
					}
				}
			}
		}
	}

	return results
}

func resolvePastNodeFunction(vnode_id, capability, socket_address string, period m2mapi.PeriodInput) m2mapi.ResolvePastNode {
	test := m2mapi.ResolvePastNode{}
	return test
}

func BodyGraphQL(byteArray []byte) []interface{} {
	var jsonBody map[string]interface{}
	if err := json.Unmarshal(byteArray, &jsonBody); err != nil {
		fmt.Printf("failed to unmarchal: %s", err)
		return nil
	}
	var values []interface{}
	for k1, v1 := range jsonBody {
		switch t1 := v1.(type) {
		case []interface{}:
			for _, v2 := range v1.([]interface{}) {
				fmt.Println("v1([]interface{}): ", v2, "type: ", t1)
				values = v1.([]interface{})
			}
		case map[string]interface{}:
			for _, v2 := range v1.(map[string]interface{}) {
				switch t2 := v2.(type) {
				case []interface{}:
					values = v2.([]interface{})
				default:
					fmt.Println("type: ", t2)
				}
			}
		default:
			fmt.Println("k1(default)", k1, ":v1(default): ", v1)
		}
	}
	return values
}

func loadEnv() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		//message.MyError(err, "loadEnv > godotenv.Load")
		log.Fatal(err)
	}
}
