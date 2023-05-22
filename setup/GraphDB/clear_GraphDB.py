import json
from py2neo import Graph, Node, Relationship
import sys
from dotenv import load_dotenv
import os

load_dotenv()
global_url = "bolt://localhost:" + os.getenv("NEO4J_GLOBAL_PORT_PYTHON")
global_graph = Graph(global_url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_GLOBAL_PASSWORD")))
global_tx = global_graph.begin()

local_url = "bolt://localhost:" + os.getenv("NEO4J_LOCAL_PORT_PYTHON")
local_graph = Graph(local_url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_LOCAL_PASSWORD")))
local_tx = local_graph.begin()

# 全レコードの削除
delete_query = "MATCH (n) DETACH DELETE n;"
global_graph.run(delete_query)
local_graph.run(delete_query)

# 全インデックスの削除
get_index_query = "SHOW INDEX"
global_indexes = global_graph.run(get_index_query).data()
for index in global_indexes:
    index_name = index["name"]
    drop_index_query = f"DROP INDEX {index_name}"
    global_graph.run(drop_index_query)
local_indexes = local_graph.run(get_index_query).data()
for index in local_indexes:
    index_name = index["name"]
    drop_index_query = f"DROP INDEX {index_name}"
    local_graph.run(drop_index_query)

global_graph.commit(global_tx)
local_graph.commit(local_tx)

print("Clear GraphDB")