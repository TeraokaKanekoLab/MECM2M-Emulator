import json
from dotenv import load_dotenv
import os
import random
import math

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/LinkProcess/Internet"

# EDGE_SERVER_NUM
EDGE_SERVER_NUM = int(os.getenv("EDGE_SERVER_NUM"))
LINK_SOCKET_ADDRESS_ROOT = "/tmp/mecm2m/link-process/"

# はじめに，元のデータを削除しておく
full_path = json_file_path + "/internet_link_process.json"
with open(full_path, 'w') as f:
    json.dump({"socket_addresses":[]}, f)

i = 0
while i <= EDGE_SERVER_NUM:
    j = i+1
    while j <= EDGE_SERVER_NUM:
        link_socket_addr_src = LINK_SOCKET_ADDRESS_ROOT + "internet_" + str(i) + "_" + str(j) + ".sock"
        link_socket_addr_dst = LINK_SOCKET_ADDRESS_ROOT + "internet_" + str(j) + "_" + str(i) + ".sock"
        full_path = json_file_path + "/internet_link_process.json"
        with open(full_path, 'r') as f:
            socket_file_data = json.load(f)
        socket_file_data["socket_addresses"].append(link_socket_addr_src)
        socket_file_data["socket_addresses"].append(link_socket_addr_dst)
        with open(full_path, 'w') as f:
            json.dump(socket_file_data, f, indent=4)
        j += 1
    i += 1
