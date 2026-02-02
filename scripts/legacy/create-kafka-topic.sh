#!/bin/bash
set -e

# Kafka configuration
KAFKA_CONTAINER="notif-kafka"
BOOTSTRAP_SERVER="localhost:9092"
TOPIC_NAME="notification-events"
PARTITIONS=10
REPLICATION_FACTOR=1
RETENTION_MS=604800000  # 7 days

echo "Creating Kafka topic: $TOPIC_NAME"

docker exec $KAFKA_CONTAINER kafka-topics --create \
    --bootstrap-server $BOOTSTRAP_SERVER \
    --topic $TOPIC_NAME \
    --partitions $PARTITIONS \
    --replication-factor $REPLICATION_FACTOR \
    --if-not-exists \
    --config retention.ms=$RETENTION_MS \
    --config compression.type=snappy

echo "Topic created successfully"

# List topics to verify
docker exec $KAFKA_CONTAINER kafka-topics --list \
    --bootstrap-server $BOOTSTRAP_SERVER

# Describe the topic
docker exec $KAFKA_CONTAINER kafka-topics --describe \
    --bootstrap-server $BOOTSTRAP_SERVER \
    --topic $TOPIC_NAME
