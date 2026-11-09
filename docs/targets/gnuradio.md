# GNU Radio

This page shows how to build gr-dmrgo, the GNU Radio 3.10 blocks of dmrgo, and how CI checks it.

gr-dmrgo is in the [gr-dmrgo](../../gr-dmrgo/README.md) directory of the repository. It is licensed under the GNU GPL version 3 or later. It loads libdmrgo, the MIT [C API](c.md) of dmrgo, at run time, and dmrgo does not depend on it.

## What you need

- GNU Radio 3.10 with its development files, CMake and a C compiler.
- Go, to build libdmrgo from `v2/capi`. The build of an installed libdmrgo needs no Go: give its path in `-DDMRGO_LIBRARY`.

## Build

From the root of a clone:

```sh
cmake -S gr-dmrgo -B gr-dmrgo/build
cmake --build gr-dmrgo/build
ctest --test-dir gr-dmrgo/build --output-on-failure
cmake --install gr-dmrgo/build
```

The build makes libdmrgo with the Makefile of `v2/capi` and `libgrdmrgo_rx.so`, the receiver of the block. The tests decode a capture of an XPR 8400 repeater, a Codec2 call, and the AMBE+2 frames of the capture on a simulated vocoder chip on a serial port and on UDP.

## Run

`gr-dmrgo/examples/dmrgo_rx_file.grc` decodes a recording of 8-bit I/Q and writes the audio to a WAV file. Compile it with `grcc` and run it:

```sh
GRC_BLOCKS_PATH=gr-dmrgo/build/prefix/share/gnuradio/grc/blocks grcc -o gr-dmrgo/build gr-dmrgo/examples/dmrgo_rx_file.grc
python3 gr-dmrgo/build/dmrgo_rx_file.py -i v2/cmd/dmrrf/testdata/xpr8400_bs_voice_weak.cu8 -o audio.wav
```

The flowgraph prints each burst with its TDMA channel, color code and call. `gr-dmrgo/examples/dmrgo_rx_gui.grc` shows the spectrum, the audio and the calls of a recording that it plays in a loop.

## CI

The `gr-dmrgo` job builds the module on Ubuntu 24.04, runs its tests, and compiles and runs the file flowgraph:

<!-- docgen:code file=.github/workflows/test.yaml symbol="jobs.gr-dmrgo.steps[Install GNU Radio:Compile and run the example flowgraph]" -->
```yaml
- name: Install GNU Radio
  run: sudo apt-get update && sudo apt-get install -y gnuradio-dev pkg-config cmake

- run: go generate ./...

- name: Build and test the module
  run: |
    cmake -S gr-dmrgo -B gr-dmrgo/build -DCMAKE_INSTALL_PREFIX="$PWD/gr-dmrgo/build/prefix"
    cmake --build gr-dmrgo/build
    ctest --test-dir gr-dmrgo/build --output-on-failure
    cmake --install gr-dmrgo/build

- name: Compile and run the example flowgraph
  run: |
    GRC_BLOCKS_PATH=gr-dmrgo/build/prefix/share/gnuradio/grc/blocks grcc -o gr-dmrgo/build gr-dmrgo/examples/dmrgo_rx_file.grc
    pkg=$(dirname "$(dirname "$(find gr-dmrgo/build/prefix -path '*/gnuradio/dmrgo/__init__.py')")")
    PYTHONPATH="$(dirname "$pkg")" LD_LIBRARY_PATH="$PWD/gr-dmrgo/build/prefix/lib" timeout 300 python3 gr-dmrgo/build/dmrgo_rx_file.py -i v2/cmd/dmrrf/testdata/xpr8400_bs_voice_weak.cu8 -o gr-dmrgo/build/audio.wav > gr-dmrgo/build/example.log
    grep -c "src . 3191868" gr-dmrgo/build/example.log
```
<!-- /docgen:code -->
