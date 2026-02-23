# Example Dockerfile. Replace with your runtime and build steps for your language/stack.
# Multi-stage example (adjust for Go, Node, Python, Rust, etc.)

# FROM node:22-alpine AS build
# WORKDIR /app
# COPY package*.json ./
# RUN npm ci
# COPY . .
# RUN npm run build

# FROM node:22-alpine
# COPY --from=build /app/dist /app
# WORKDIR /app
# CMD ["node", "index.js"]

# Placeholder so the template has a Dockerfile to customize
FROM alpine:3.19
RUN echo "Replace this Dockerfile with your app's build and runtime."
CMD ["echo", "Add your app image."]
