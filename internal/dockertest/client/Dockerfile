FROM debian:bookworm-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    zsh \
    git \
    python3 \
    openssh-client \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Clone Zsh plugins
RUN git clone https://github.com/zsh-users/zsh-autosuggestions.git /opt/zsh-autosuggestions && \
    git clone https://github.com/zsh-users/zsh-syntax-highlighting.git /opt/zsh-syntax-highlighting

# Install Mutagen
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "arm64" ]; then MUTAGEN_ARCH="arm64"; else MUTAGEN_ARCH="amd64"; fi && \
    curl -L -o /tmp/mutagen.tar.gz "https://github.com/mutagen-io/mutagen/releases/download/v0.17.2/mutagen_linux_${MUTAGEN_ARCH}_v0.17.2.tar.gz" && \
    mkdir -p /opt/mutagen && \
    tar -C /opt/mutagen -xzf /tmp/mutagen.tar.gz && \
    ln -s /opt/mutagen/mutagen /usr/local/bin/mutagen && \
    ln -s /opt/mutagen/mutagen-agents.tar.gz /usr/local/bin/mutagen-agents.tar.gz && \
    rm /tmp/mutagen.tar.gz

# Copy compile target sssh
COPY client/sssh /usr/local/bin/sssh
RUN chmod +x /usr/local/bin/sssh

# Copy mock docker and podman scripts
COPY server/mock_docker.sh /usr/local/bin/docker
COPY server/mock_podman.sh /usr/local/bin/podman
RUN chmod +x /usr/local/bin/docker /usr/local/bin/podman

# Set up test user
RUN useradd -ms /bin/zsh testuser

# Copy test script
COPY client/test_interactive.py /home/testuser/test_interactive.py
RUN chown testuser:testuser /home/testuser/test_interactive.py

USER testuser
WORKDIR /home/testuser

# Set up SSH directory
RUN mkdir -p /home/testuser/.ssh && chmod 700 /home/testuser/.ssh

