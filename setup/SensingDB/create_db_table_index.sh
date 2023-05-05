# !/bin/bash

#MYSQLをインストールしてから，インストールを始める前に，SensingDBにデータベースとテーブルとインデックスを付与する
printf "\e[1;31m \n *** CREATE DATABASE, TABLE AND INDEX IN MYSQL *** \e[0m"

# ホーム下に用意した.envファイルを読み込む
source $HOME/.env

printf "\e[1;31m \n1. CREATE DATABASE \e[0m"
# データベースの作成
mysql -u root -p -e "CREATE DATABASE ${MYSQL_DB};"

printf "\e[1;31m \n2. CREATE TABLE \e[0m"
# テーブルの作成
mysql -u root -p -e $MYSQL_DB "CREATE TABLE ${MYSQL_TABLE}(PNodeID VARCHAR(20), Capability VARCHAR(20), Timestamp VARCHAR(30), Value VARCHAR(20), PSinkID VARCHAR(20), ServerID VARCHAR(20), Lat VARCHAR(20), Lon VARCHAR(20), VNodeID VARCHAR(20), VPointID VARCHAR(20));"

printf "\e[1;31m \n3. CREATE INDEX \e[0m"
# テーブルの作成
mysql -u root -p -e $MYSQL_DB "CREATE UNIQUE INDEX prim_index on ${MYSQL_TABLE}(PNodeID, Capability, Timestamp);"