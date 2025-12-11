# SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
#
# SPDX-License-Identifier: MIT

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# GoReleaser will build the binary and place it in the Docker build context
# as 'boot-service' for each target platform.
COPY metadata-service /usr/local/bin/metadata-service

USER nonroot:nonroot
EXPOSE 8080 9090

ENTRYPOINT ["/usr/local/bin/metadata-service"]
CMD ["serve"]
