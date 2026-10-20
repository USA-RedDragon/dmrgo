# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

set(DMRGO_PYTHON_SOURCES __init__.py ambe.py libdmrgo.py rx.py)

GR_PYTHON_INSTALL(FILES ${DMRGO_PYTHON_SOURCES} DESTINATION ${GR_PYTHON_DIR}/gnuradio/dmrgo)

foreach(f ${DMRGO_PYTHON_SOURCES})
    configure_file(${f} ${PROJECT_BINARY_DIR}/test_modules/gnuradio/dmrgo/${f} COPYONLY)
endforeach()

include(GrTest)
set(GR_TEST_ENVIRONS "DMRGO_LIB=${DMRGO_LIB_FILE}" "DMRGO_TESTDATA=${CMAKE_CURRENT_SOURCE_DIR}/../../../v2")
GR_ADD_TEST(qa_rx ${PYTHON_EXECUTABLE} -B ${CMAKE_CURRENT_SOURCE_DIR}/qa_rx.py)
