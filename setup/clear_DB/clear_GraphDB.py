import json
from py2neo import Graph, Node, Relationship
import sys
from dotenv import load_dotenv
import os

load_dotenv()
url = "bolt://localhost:" + os.getenv("NEO4J_PORT")
graph = Graph(url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_PASSWORD")))

tx = graph.begin()

#全レコードの削除
delete_query = "MATCH (n) DETACH DELETE n;"
graph.run(delete_query)

#全インデックスの削除
get_index_query = "SHOW INDEX"
indexes = graph.run(get_index_query).data()
for index in indexes:
    index_name = index["name"]
    drop_index_query = f"DROP INDEX {index_name}"
    graph.run(drop_index_query)

graph.commit(tx)