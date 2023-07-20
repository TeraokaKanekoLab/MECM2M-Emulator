import pymysql
from dotenv import load_dotenv
import os

load_dotenv()

host = "localhost"
user = os.getenv("MYSQL_USERNAME")
password = os.getenv("MYSQL_PASSWORD")
global_db = os.getenv("MYSQL_GLOBAL_DB")
local_db = os.getenv("MYSQL_LOCAL_DB")
table = os.getenv("MYSQL_TABLE")

edge_server_num = int(os.getenv("EDGE_SERVER_NUM"))

# Global SensingDBに接続
global_connection = pymysql.connect(host=host, user=user, password=password, database=global_db)
global_cursor = global_connection.cursor()

# Local SensingDBに接続
local_connection = pymysql.connect(host=host, user=user, password=password, database=local_db)
local_cursor = local_connection.cursor()

# データベースを新たに作成する
create_database_query = f"CREATE DATABASE IF NOT EXISTS {global_db};"
global_cursor.execute(create_database_query)
create_database_query = f"CREATE DATABASE IF NOT EXISTS {local_db};"
local_cursor.execute(create_database_query)

use_database_cursor_query = f"USE {global_db}"
global_cursor.execute(use_database_cursor_query)
use_database_cursor_query = f"USE {local_db}"
local_cursor.execute(use_database_cursor_query)

# テーブルを新たに作成する (global)
create_table_query = f"CREATE TABLE IF NOT EXISTS {table}(PNodeID VARCHAR(20), Capability VARCHAR(20), Timestamp VARCHAR(30), Value DECIMAL(5,2), PSinkID VARCHAR(20), Lat DECIMAL(6,4), Lon DECIMAL(7,4));"
global_cursor.execute(create_table_query)

# インデックスを新たに作成する (global)
create_index_query = f"CREATE UNIQUE INDEX prim_index on {table}(PNodeID, Capability, Timestamp);"
global_cursor.execute(create_index_query)

# テーブル・インデックスを新たに作成する (local)
i = 1
while i <= edge_server_num:
    create_table_query = f"CREATE TABLE IF NOT EXISTS {table}_{i}(PNodeID VARCHAR(20), Capability VARCHAR(20), Timestamp VARCHAR(30), Value DECIMAL(5,2), PSinkID VARCHAR(20), Lat DECIMAL(6,4), Lon DECIMAL(7,4));"
    local_cursor.execute(create_table_query)
    create_index_query = f"CREATE UNIQUE INDEX prim_index on {table}_{i}(PNodeID, Capability, Timestamp);"
    local_cursor.execute(create_index_query)
    i += 1

# 変更をコミットして，接続を切断する
global_connection.commit()
local_connection.commit()
global_cursor.close()
local_cursor.close()
global_connection.close()
local_connection.close()

print("Create Global/Local SensingDB")