# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

cmake_minimum_required(VERSION 3.16)
project(gr-dmrgo VERSION 0.1.0 LANGUAGES C CXX)

find_package(Gnuradio "3.10" REQUIRED COMPONENTS runtime)
include(GNUInstallDirs)
include(GrPython)

set(DMRGO_CAPI_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../v2/capi" CACHE PATH "Directory of the dmrgo C API")
set(DMRGO_LIBRARY "" CACHE FILEPATH "Path of a built libdmrgo shared library")

if(DMRGO_LIBRARY)
    set(DMRGO_LIB_FILE "${DMRGO_LIBRARY}")
else()
    find_program(GO_EXECUTABLE go REQUIRED)
    find_program(MAKE_EXECUTABLE NAMES gmake make REQUIRED)
    file(STRINGS "${DMRGO_CAPI_DIR}/abi.txt" DMRGO_ABI_LINE REGEX "^abi ")
    string(REGEX REPLACE "^abi " "" DMRGO_ABI "${DMRGO_ABI_LINE}")
    set(DMRGO_LIB_DIR "${CMAKE_CURRENT_BINARY_DIR}/libdmrgo")
    set(DMRGO_LIB_FILE "${DMRGO_LIB_DIR}/libdmrgo.so.${DMRGO_ABI}")
    file(GLOB DMRGO_CAPI_SOURCES "${DMRGO_CAPI_DIR}/*.go" "${DMRGO_CAPI_DIR}/*.c" "${DMRGO_CAPI_DIR}/*.h")
    add_custom_command(
        OUTPUT "${DMRGO_LIB_FILE}" "${DMRGO_LIB_DIR}/libdmrgo.so"
        COMMAND "${MAKE_EXECUTABLE}" -C "${DMRGO_CAPI_DIR}" "BUILD=${DMRGO_LIB_DIR}" "${DMRGO_LIB_DIR}/libdmrgo.so"
        DEPENDS ${DMRGO_CAPI_SOURCES}
        COMMENT "Building libdmrgo"
        VERBATIM)
    add_custom_target(libdmrgo ALL DEPENDS "${DMRGO_LIB_FILE}")
    install(FILES "${DMRGO_LIB_FILE}" "${DMRGO_LIB_DIR}/libdmrgo.so" DESTINATION ${CMAKE_INSTALL_LIBDIR})
endif()

add_library(grdmrgo_rx SHARED lib/grdmrgo_rx.c)
target_include_directories(grdmrgo_rx PRIVATE "${DMRGO_CAPI_DIR}" lib)
target_link_libraries(grdmrgo_rx PRIVATE "${DMRGO_LIB_FILE}" m)
set_target_properties(grdmrgo_rx PROPERTIES C_STANDARD 11 LIBRARY_OUTPUT_DIRECTORY "${PROJECT_BINARY_DIR}" INSTALL_RPATH "${CMAKE_INSTALL_FULL_LIBDIR}")
target_compile_options(grdmrgo_rx PRIVATE -Wall -Wextra)
if(TARGET libdmrgo)
    add_dependencies(grdmrgo_rx libdmrgo)
endif()

set(GR_PKG_DATA_DIR ${CMAKE_INSTALL_DATADIR}/${CMAKE_PROJECT_NAME})

enable_testing()
add_subdirectory(python/dmrgo)
add_subdirectory(grc)
install(FILES examples/dmrgo_rx_file.grc examples/dmrgo_rx_gui.grc DESTINATION ${GR_PKG_DATA_DIR}/examples)
