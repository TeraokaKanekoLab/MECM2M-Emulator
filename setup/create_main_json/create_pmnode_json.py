import json
from dotenv import load_dotenv
import os
import math
import random
from datetime import datetime
import string
import ipaddress

# PMNode
# ---------------
## Data Property
## * PNodeID
## * PNode Type (デバイスとクラスの対応づけ)
## * VNode Module ID (VNode moduleの実行系ファイル)
## * Socket Address (VNode のソケットファイル，本来はIP:Port)
## * Lat Lon
## * Capability
## * Credential
## * Session Key
## * Description 
## * Home IPv6 Address
## * Update Time
## * Velocity
## * Acceleration
## * Direction
# ---------------
## Object Property
## * aggregates (PSink->PNode)
## * isConnectedTo (PNode->PSink)
## * contains (PArea->PNode)
## * isInstalledIn (PNode->PArea)

# VMNode
# ---------------
## Data Property
## * VNodeID
## * Socket Address (VNode のソケットファイル，本来はIP:Port)
## * Software Module (VNode moduleの実行系ファイル)
## * Description
## * Representative Socket Address (VMNodeR のソケットファイル，本来はIP:Port)
# ---------------
## Object Property
## * isVirtualizedBy (PNode->VNode)
## * isPhysicalizedBy (VNode->PNode)
## * contains (PArea->VNode)
## * isInstalledIn (VNode->PArea)

def generate_random_ipv6():
    random_int = random.getrandbits(128)
    ipv6 = ipaddress.IPv6Address(random_int)
    return str(ipv6)

# session key 生成
def generate_random_string(length):
    # 文字列に使用する文字を定義
    letters = string.ascii_letters + string.digits

    result_str = ''.join(random.choice(letters) for i in range(length))
    return result_str

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/setup/GraphDB/config/"

# VMNODEH_BASE_PORT
# PSINK_NUM_PER_AREA
# MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
# AREA_WIDTH
# IP_ADDRESS
VMNODE_BASE_PORT = int(os.getenv("VMNODE_BASE_PORT"))
PSINK_NUM_PER_AREA = float(os.getenv("PSINK_NUM_PER_AREA"))
MIN_LAT = float(os.getenv("MIN_LAT"))
MAX_LAT = float(os.getenv("MAX_LAT"))
MIN_LON = float(os.getenv("MIN_LON"))
MAX_LON = float(os.getenv("MAX_LON"))
AREA_WIDTH = float(os.getenv("AREA_WIDTH"))
IP_ADDRESS = os.getenv("IP_ADDRESS")

lineStep = AREA_WIDTH
forint = 1000

data = {"pmnodes":[]}

# 始点となるArea
swLat = MIN_LAT
neLat = swLat + lineStep

# label情報
label_lat = 0
label_lon = 0

# ID用のindex
id_index = 0

# psink用のindex
psink_index = 0

'''
#PNTypeをあらかじめ用意
pn_types = ["Toyota", "Matsuda", "Nissan"]
capabilities = {"Toyota":"Prius", "Matsuda":"Road-Star", "Nissan":"Selena"}
vmnode_pmnode_relation = {}
for i in range(len(pn_types)):
    vmnode_pmnode_relation[pn_types[i]] = []
'''

# VMNode/initial_environment.json の初期化
vmnode_dir_path = os.getenv("PROJECT_PATH") + "/VMNode/"
initial_environment_file = vmnode_dir_path + "initial_environment.json"
port_array = {
    "ports": []
}
with open(initial_environment_file, 'w') as file:
    json.dump(port_array, file, indent=4)


#左下からスタートし，右へ進んでいく
#端まで到達したら一段上へ
while neLat <= MAX_LAT:
    swLon = MIN_LON
    neLon = swLon + lineStep
    label_lon = 0
    while neLon <= MAX_LON:
        i = 0
        while i < PSINK_NUM_PER_AREA:
            psink_label = "PS" + str(psink_index)
            data["pmnodes"].append({"pmnode":{}, "vmnode":{}})

            # PMNode情報の追加
            parea_label = "PA" + str(label_lat) + ":" + str(label_lon)
            pmnode_label = "PMN" + str(id_index)
            pmnode_id = str(int(0b0100 << 60) + id_index)
            vmnode_id = str(int(0b1100 << 60) + id_index)
            pnode_type = "Car"
            vnode_module = os.getenv("PROJECT_PATH") + "/VMNode/main"
            vmnode_port = int(VMNODE_BASE_PORT) + id_index
            vnode_socket_adress = IP_ADDRESS + ":" + str(vmnode_port)
            pmnode_lat = random.uniform(swLat, neLat)
            pmnode_lon = random.uniform(swLon, neLon)
            capability = ["Accel", "Brake", "Handle"]
            credential = "YES"
            session_key = generate_random_string(10)
            pmnode_description = "Description:" + pmnode_label
            home_ipv6_address = IP_ADDRESS
            update_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            velocity = 50.50
            acceleration = 10.10
            direction = "North"
            pmnode_dict = {
                "property-label": "PMNode",
                "relation-label": {
                    "PSink": psink_label,
                    "PArea": parea_label,
                },
                "data-property": {
                    "Label": pmnode_label,
                    "PNodeID": pmnode_id,
                    "PNodeType": pnode_type,
                    "VNodeModule": vnode_module,
                    "SocketAddress": "",
                    "Position": [round(pmnode_lat, 4), round(pmnode_lon, 4)],
                    "Capability": capability,
                    "Credential": credential,
                    "SessionKey": session_key,
                    "Description": pmnode_description,
                    "HomeIPv6Address": home_ipv6_address,
                    "UpdateTime": update_time,
                    "Velocity": velocity,
                    "Acceleration": acceleration,
                    "Direction": direction
                },
                "object-property": [
                    {
                        "from": {
                            "property-label": "PMNode",
                            "data-property": "Label",
                            "value": pmnode_label
                        },
                        "to": {
                            "property-label": "PSink",
                            "data-property": "Label",
                            "value": psink_label
                        }, 
                        "type": "isConnectedTo"
                    },
                    {
                        "from": {
                            "property-label": "PSink",
                            "data-property": "Label",
                            "value": psink_label
                        },
                        "to": {
                            "property-label": "PMNode",
                            "data-property": "Label",
                            "value": pmnode_label
                        }, 
                        "type": "aggregates"
                    },
                    {
                        "from": {
                            "property-label": "PArea",
                            "data-property": "Label",
                            "value": parea_label
                        },
                        "to": {
                            "property-label": "PMNode",
                            "data-property": "Label",
                            "value": pmnode_label
                        }, 
                        "type": "contains"
                    },
                    {
                        "from": {
                            "property-label": "PMNode",
                            "data-property": "Label",
                            "value": pmnode_label
                        },
                        "to": {
                            "property-label": "PArea",
                            "data-property": "Label",
                            "value": parea_label
                        }, 
                        "type": "isInstalledIn"
                    },
                ]
            }
            data["pmnodes"][-1]["pmnode"] = pmnode_dict

            # VMNode情報の追加
            parea_label = "PA" + str(label_lat) + ":" + str(label_lon)
            vmnode_label = "VMN" + str(id_index)
            vmnoder_socket_address = "192.168.11.11:13000"
            vmnode_description = "Description:" + vmnode_label
            vmnode_dict = {
                "property-label": "VMNode",
                "relation-label": {
                    "PArea": parea_label
                },
                "data-property": {
                    "Label": vmnode_label,
                    "VNodeID": vmnode_id,
                    "SocketAddress": vnode_socket_adress,
                    "SoftwareModule": vnode_module,
                    "VMNodeRSocketAddress": vmnoder_socket_address,
                    "Description": vmnode_description
                },
                "object-property": [
                    {
                        "from": {
                            "property-label": "PMNode",
                            "data-property": "Label",
                            "value": pmnode_label
                        },
                        "to": {
                            "property-label": "VMNode",
                            "data-property": "Label",
                            "value": vmnode_label
                        }, 
                        "type": "isVirtualizedBy"
                    },
                    {
                        "from": {
                            "property-label": "VMNode",
                            "data-property": "Label",
                            "value": vmnode_label
                        },
                        "to": {
                            "property-label": "PMNode",
                            "data-property": "Label",
                            "value": pmnode_label
                        }, 
                        "type": "isPhysicalizedBy"
                    },
                    {
                        "from": {
                            "property-label": "PArea",
                            "data-property": "Label",
                            "value": parea_label
                        },
                        "to": {
                            "property-label": "VMNode",
                            "data-property": "Label",
                            "value": vmnode_label
                        }, 
                        "type": "contains"
                    },
                    {
                        "from": {
                            "property-label": "VMNode",
                            "data-property": "Label",
                            "value": vmnode_label
                        },
                        "to": {
                            "property-label": "PArea",
                            "data-property": "Label",
                            "value": parea_label
                        }, 
                        "type": "isInstalledIn"
                    },
                ]
            }
            data["pmnodes"][-1]["vmnode"] = vmnode_dict

            # VMNode/initial_environment.jsonに初期環境に配置されるVMNodeのポート番号を格納
            initial_environment_file = vmnode_dir_path + "initial_environment.json"
            with open(initial_environment_file, 'r') as file:
                ports_data = json.load(file)
            ports_data["ports"].append(vmnode_port)
            with open(initial_environment_file, 'w') as file:
                json.dump(ports_data, file, indent=4)
            
            id_index += 1
            i += 1
            psink_index += 1
        label_lon += 1
        swLon = ((swLon*forint) + (lineStep*forint)) / forint
        neLon = ((neLon*forint) + (lineStep*forint)) / forint
    label_lat += 1
    swLat = ((swLat*forint) + (lineStep*forint)) / forint
    neLat = ((neLat*forint) + (lineStep*forint)) / forint

pmnode_json = json_file_path + "/config_main_pmnode.json"
with open(pmnode_json, 'w') as f:
    json.dump(data, f, indent=4)