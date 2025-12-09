#!/bin/bash

source .env
docker run \
	--name $POSTGRES_DB \
	-e POSTGRES_USER=$POSTGRES_USER \
	-e POSTGRES_DB=$POSTGRES_DB \
	-e POSTGRES_PASSWORD=$POSTGRES_PASSWORD \
	-p 5432:5432 \
	-d postgres
