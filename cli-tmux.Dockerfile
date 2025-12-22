# Assumes a local copy of gemini-cli-sandbox


FROM gemini-cli-sandbox
USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
  tmux \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

USER node