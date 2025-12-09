from golang:1.25-trixie
workdir /app
copy . .
run make
cmd ["build/ivgpu-schedule"]
