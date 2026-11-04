
MEMORY
{
    FLASH_TEXT (rw) : ORIGIN = 0x00000000, LENGTH = 4M
    RAM (xrw)       : ORIGIN = 0x20000000, LENGTH = 4M
}

_stack_size = 32K;

INCLUDE "targets/arm.ld"
