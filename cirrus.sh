#!/bin/bash

# Welcome Message
echo "Welcome to the Docker Compose Generator!"
echo "This script will help you set up a local environment with multiple Docker containers."
echo ""
echo "You'll be prompted to enter the number of minion services you want to create."
echo "An additional service called 'bastion' will also be created."
echo ""
echo "After the setup, you'll receive SSH commands to connect to your minion containers."
echo ""
echo "Help: You can enter a number greater than 0 for the number of minion services."
echo ""

# Prompt for the number of minion services
read -p "Enter the number of minion services: " num_services

# Validate input to ensure it's a positive integer
if ! [[ "$num_services" =~ ^[0-9]+$ ]] || [ "$num_services" -le 0 ]; then
    echo "Error: Please enter a valid positive integer greater than zero."
    exit 1
fi

# Create the docker-compose.yml file
echo "version: '3.8'" > docker-compose.yml
echo "services:" >> docker-compose.yml

# Add the bastion service
echo "  bastion:" >> docker-compose.yml
echo "    container_name: bastion" >> docker-compose.yml
echo "    hostname: bastion" >> docker-compose.yml
echo "    image: schooleon/minion" >> docker-compose.yml  # Updated image for bastion service
echo "    ports:" >> docker-compose.yml
echo "      - 220:22" >> docker-compose.yml  # Map host port 220 to container port 22

# Loop to create minion services based on the entered number
for ((i=1; i<=num_services; i++)); do
    service_name="minion-$i"
    host_port=$((220 + i))  # Start host port from 221 and increment

    echo "  $service_name:" >> docker-compose.yml
    echo "    container_name: $service_name" >> docker-compose.yml
    echo "    hostname: $service_name" >> docker-compose.yml
    echo "    image: schooleon/minion" >> docker-compose.yml  # Updated image
    echo "    ports:" >> docker-compose.yml
    echo "      - $host_port:22" >> docker-compose.yml  # Map host port to container port 22
done

# Final message
echo "docker-compose.yml has been generated with $num_services minion services and a bastion service."
echo ""

# Start the containers
echo "Starting Docker containers..."
docker compose up -d

# Wait for a few seconds to allow containers to start
sleep 5

# Fetch container IPs and update /etc/hosts
echo "Fetching container IPs and updating /etc/hosts."
for ((i=1; i<=num_services; i++)); do
    service_name="minion-$i"
    container_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$service_name")
    
    # If entry exists, remove it
    sudo sed -i.bak "/\s*$service_name\s*/d" /etc/hosts

    # Add new entry to /etc/hosts
    echo "$container_ip $service_name" | sudo tee -a /etc/hosts > /dev/null
done

# Handle bastion service
bastion_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' bastion)

# If entry exists, remove it
sudo sed -i.bak "/\s*bastion\s*/d" /etc/hosts

# Add bastion's IP to /etc/hosts
echo "$bastion_ip bastion" | sudo tee -a /etc/hosts > /dev/null

# SSH Command Guide
echo "You can use the following SSH commands to log into your containers:"
echo ""
echo "ssh minion@bastion -- creds: minion:minion"
for ((i=1; i<=num_services; i++)); do
    service_name="minion-$i"
    echo "ssh minion@$service_name -- creds: minion:minion"
done