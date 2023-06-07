import pymysql
from dotenv import load_dotenv
import os

load_dotenv()

host = "localhost"
user = os.getenv("MYSQL_USERNAME")
password = os.getenv("MYSQL_PASSWORD")
database = os.getenv("MYSQL_LOCAL_DB")
table = os.getenv("MYSQL_TABLE")

# DBに接続
connection = pymysql.connect(host=host, user=user, password=password, database=database)
cursor = connection.cursor()

# データベースを新たに作成する
create_database_query = f"CREATE DATABASE IF NOT EXISTS {database};"
cursor.execute(create_database_query)

use_database_cursor_query = f"USE {database}"
cursor.execute(use_database_cursor_query)

# テーブルを新たに作成する
create_table_query = f"CREATE TABLE IF NOT EXISTS {table}(PNodeID VARCHAR(20), Capability VARCHAR(20), Timestamp VARCHAR(30), Value DECIMAL(5,2), PSinkID VARCHAR(20), Lat DECIMAL(6,4), Lon DECIMAL(7,4));"
cursor.execute(create_table_query)

# インデックスを新たに作成する
create_index_query = f"CREATE UNIQUE INDEX prim_index on {table}(PNodeID, Capability, Timestamp);"
cursor.execute(create_index_query)

# 変更をコミットして，接続を切断する
connection.commit()
cursor.close()
connection.close()

print("Create SensingDB")