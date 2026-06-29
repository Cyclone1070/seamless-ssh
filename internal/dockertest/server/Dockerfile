FROM debian:bookworm-slim

# Install system dependencies (SSH server + netcat)
RUN apt-get update && apt-get install -y \
    openssh-server \
    sudo \
    netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*

# Set up SSH daemon
RUN mkdir /var/run/sshd
RUN sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config
RUN sed -i 's/#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config

# Set up test user
RUN useradd -ms /bin/bash testuser && \
    echo "testuser ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers
USER testuser
WORKDIR /home/testuser

# Set up SSH directory
RUN mkdir -p /home/testuser/.ssh && chmod 700 /home/testuser/.ssh

# Copy mock docker and podman scripts
USER root
COPY server/mock_docker.sh /usr/local/bin/docker
COPY server/mock_podman.sh /usr/local/bin/podman
RUN chmod +x /usr/local/bin/docker /usr/local/bin/podman

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D"]
