import json
from dotenv import load_dotenv
import os
import random
import ipaddress

def generate_random_ipv6():
    random_int = random.getrandbits(128)
    ipv6 = ipaddress.IPv6Address(random_int)
    return str(ipv6)

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/Main/config/json_files"

#EDGE_SERVER_NUM
EDGE_SERVER_NUM = 3

data = {"servers":{}}
ipv6 = []

for i in range(EDGE_SERVER_NUM+1):
    ipv6_address = generate_random_ipv6()
    ipv6.append(ipv6_address)
data["servers"]["ipv6"] = ipv6

data["servers"]["server"] = []

#S0をCloud Serverとし，S1以降をEdge Serverとする
for i in range(EDGE_SERVER_NUM+1):
    server = data["servers"]["server"]
    label = "S" + str(i)

    server_dict = {
        "property-label": "Server",
        "data-property": {
            "Label": label,
            "IPv6Address": ipv6[i],
            "ServedIPv6Pref": str(i),
            "Description": "Server" + label
        },
        "object-property": [
        
        ]
    }
    object_properties = server_dict["object-property"]
    if i > 0:
        #Edge Serverのobject propertyを付加
        cloud_label = "S0"
        isLowerOf_object_property = {
            "from": {
                "property-label": "Server",
                "data-property": "Label",
                "value": label
            },
            "to": {
                "property-label": "Server",
                "data-property": "Label",
                "value": cloud_label
            },
            "type": "isLowerOf"
        }
        isUpperOf_object_property = {
            "from": {
                "property-label": "Server",
                "data-property": "Label",
                "value": cloud_label
            },
            "to": {
                "property-label": "Server",
                "data-property": "Label",
                "value": label
            },
            "type": "isUpperOf"
        }
        object_properties.append(isLowerOf_object_property)
        object_properties.append(isUpperOf_object_property)

    server.append(server_dict)


server_json = json_file_path + "/config_main_server.json"
with open(server_json, 'w') as f:
    json.dump(data, f, indent=4)

#ソケットファイル群ファイルもここで作る
socket_files_dir_path = os.getenv("PROJECT_PATH") + "/MECServer/Server/socket_files"
if not os.path.exists(socket_files_dir_path):
    os.makedirs(socket_files_dir_path)

for i in range(EDGE_SERVER_NUM):
    socket_data = {
        "m2mApi": "/tmp/mecm2m/svr_" + str(i+1) + "_m2mapi.sock",
        "localMgr": "/tmp/mecm2m/svr_" + str(i+1) + "_localmgr.sock",
        "pnodeMgr": "/tmp/mecm2m/svr_" + str(i+1) + "_pnodemgr.sock",
        "aaa": "/tmp/mecm2m/svr_" + str(i+1) + "_aaa.sock",
        "localRepo": "/tmp/mecm2m/svr_" + str(i+1) + "_localrepo.sock",
        "graphDB": "/tmp/mecm2m/svr_" + str(i+1) + "_graphdb.sock",
        "sensingDB": "/tmp/mecm2m/svr_" + str(i+1) + "_sensingdb.sock"
    }
    socket_json = os.getenv("PROJECT_PATH") + "/MECServer/Server/socket_files/server_" + str(i+1) + ".json"
    with open(socket_json, 'w') as f:
        json.dump(socket_data, f, indent=4)

