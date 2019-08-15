# Read temperature from display of sauna

# Install server

go get github.com/BurntSushi/toml
go get github.com/dhowden/raspicam
go get github.com/op/go-logging
go get github.com/otiai10/gosseract
go get gocv.io/x/gocv

go build server.go

## Create config file

config.toml with the following content

```
BackendURL = "http://127.0.0.1:3000/temps"
IftttURL = "https://maker.ifttt.com/trigger/<your trigger>/with/key/<your api key>"
LogLevel = "DEBUG" # INFO,NOTICE,WARNING,ERROR,CRITICAL
```

# Install opencv

apt install python3-pip
pip3 install numpy

wget -O opencv.zip https://github.com/opencv/opencv/archive/4.1.1.zip && wget -O opencv_contrib.zip https://github.com/opencv/opencv_contrib/archive/4.1.1.zip

unzip opencv.zip && unzip opencv_contrib.zip && mv opencv-4.1.1 opencv && mv opencv_contrib-4.1.1 opencv_contrib
rm -f opencv.zip opencv_contrib.zip
cd ~/opencv && mkdir build && cd build

cmake -D CMAKE_BUILD_TYPE=RELEASE \
    -D CMAKE_INSTALL_PREFIX=/usr/local \
    -D OPENCV_EXTRA_MODULES_PATH=~/opencv_contrib/modules \
    -D ENABLE_NEON=OFF \
    -D ENABLE_VFPV3=OFF \
    -D BUILD_TESTS=OFF \
    -D OPENCV_GENERATE_PKGCONFIG=ON \
    -D OPENCV_ENABLE_NONFREE=ON \
    -D INSTALL_PYTHON_EXAMPLES=OFF \
    -D BUILD_EXAMPLES=OFF ..

cd ~/opencv/build
make -j4

make install
ldconfig

# configure pkg-config
mkdir /usr/include/opencv4
cp unix-install/opencv4.pc /usr/include/opencv4/
pkg-config --cflags -I/usr/include/opencv4/opencv -I/usr/include/opencv4 opencv4


# Install tesseract

dpkg - -configure –a
apt-get install tesseract-ocr



vi ./modules/core/CMakeFiles/opencv_core.dir/link.txt # Add -latomic