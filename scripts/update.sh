pid=$(lsof -ti:8080) && [ -n "$pid" ] && kill "$pid"
cd opendocket
cp opendocket.d* ../
git pull
cp ../opendocket.d* .
./scripts/install.sh
make build
screen -dmS web make server
