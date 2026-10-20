# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

cmake_minimum_required(VERSION 3.16)
project(gr-dmrgo VERSION 0.1.0 LANGUAGES CXX)

find_package(Gnuradio "3.10" REQUIRED COMPONENTS runtime)
include(GNUInstallDirs)
include(GrPython)

set(DMRGO_CAPI_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../v2/capi" CACHE PATH "Directory of the dmrgo C API")
set(DMRGO_LIBRARY "" CACHE FILEPATH "Path of a built libdmrgo shared library")

if(DMRGO_LIBRARY)
    set(DMRGO_LIB_FILE "${DMRGO_LIBRARY}")
else()
    find_program(GO_EXECUTABLE go REQUIRED)
    set(DMRGO_LIB_FILE "${CMAKE_CURRENT_BINARY_DIR}/libdmrgo/libdmrgo.so")
    file(GLOB DMRGO_CAPI_SOURCES "${DMRGO_CAPI_DIR}/*.go" "${DMRGO_CAPI_DIR}/*.c" "${DMRGO_CAPI_DIR}/*.h")
    add_custom_command(
        OUTPUT "${DMRGO_LIB_FILE}"
        COMMAND "${GO_EXECUTABLE}" build -C "${DMRGO_CAPI_DIR}" -buildmode=c-shared -o "${DMRGO_LIB_FILE}" .
        DEPENDS ${DMRGO_CAPI_SOURCES}
        COMMENT "Building libdmrgo"
        VERBATIM)
    add_custom_target(libdmrgo ALL DEPENDS "${DMRGO_LIB_FILE}")
    install(FILES "${DMRGO_LIB_FILE}" DESTINATION ${CMAKE_INSTALL_LIBDIR})
endif()

set(GR_PKG_DATA_DIR ${CMAKE_INSTALL_DATADIR}/${CMAKE_PROJECT_NAME})

enable_testing()
add_subdirectory(python/dmrgo)
add_subdirectory(grc)
install(FILES examples/dmrgo_rx_file.grc DESTINATION ${GR_PKG_DATA_DIR}/examples)
