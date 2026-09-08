cmake_minimum_required(VERSION 3.24)
project(sensorium_cuda LANGUAGES CXX)
include(CTest)

option(SENSORIUM_CPU_CHECKS_ONLY "Build serial numerical source checks, not CUDA" OFF)

if(SENSORIUM_CPU_CHECKS_ONLY)
    add_executable(sensorium_cpu_math tests/host_math_test.cpp)
    target_compile_features(sensorium_cpu_math PRIVATE cxx_std_20)
    target_include_directories(sensorium_cpu_math PRIVATE tests/cpu_runtime)
    target_compile_options(sensorium_cpu_math PRIVATE
        $<$<CXX_COMPILER_ID:GNU,Clang>:-ffp-contract=off;-fno-fast-math>)
    add_test(NAME cpu_source_numerics COMMAND sensorium_cpu_math)
    message(STATUS "CPU checks only: no CUDA compilation, execution, or Metal parity testing")
    return()
endif()

enable_language(CUDA)
find_package(CUDAToolkit REQUIRED)
add_library(sensorium_cuda SHARED manifold.cu ops.cu)
set_target_properties(sensorium_cuda PROPERTIES
    CXX_STANDARD 20 CXX_STANDARD_REQUIRED YES
    CUDA_STANDARD 20 CUDA_STANDARD_REQUIRED YES
    CUDA_SEPARABLE_COMPILATION ON
    CUDA_RESOLVE_DEVICE_SYMBOLS ON
    POSITION_INDEPENDENT_CODE ON
    CUDA_RUNTIME_LIBRARY Shared
    LIBRARY_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}")
target_link_libraries(sensorium_cuda PRIVATE CUDA::cudart)
target_compile_options(sensorium_cuda PRIVATE
    $<$<COMPILE_LANGUAGE:CUDA>:--fmad=false;--ftz=false;--prec-div=true;--prec-sqrt=true;-lineinfo>)
if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
    target_compile_options(sensorium_cuda PRIVATE
        $<$<COMPILE_LANGUAGE:CUDA>:-Xcompiler=-ffp-contract=off,-fno-fast-math>)
endif()

if(BUILD_TESTING)
    add_executable(sensorium_cuda_tests tests/device_tests.cpp)
    target_compile_features(sensorium_cuda_tests PRIVATE cxx_std_20)
    target_link_libraries(sensorium_cuda_tests PRIVATE sensorium_cuda CUDA::cudart)
    add_test(NAME cuda_device_numerics COMMAND sensorium_cuda_tests)
    set_tests_properties(cuda_device_numerics PROPERTIES SKIP_RETURN_CODE 77)
endif()
