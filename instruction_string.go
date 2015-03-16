// generated by stringer -type=Instruction; DO NOT EDIT

package avr

import "fmt"

const _Instruction_name = "ADCADDADIWANDANDIASRBCLRBLDBRBCBRBSBREAKBSETBSTCALLCBICOMCPCPCCPICPSEDECDESEICALLEIJMPELPM_R0ELPMELPM_INCEORFMULFMULSFMULSUICALLIJMPININCJMPLACLASLATLD_XLD_X_INCLD_X_DECLD_YLD_Y_INCLD_Y_DECLDD_YLD_ZLD_Z_INCLD_Z_DECLDD_ZLDILDSLDS_SHORTLPM_R0LPMLPM_INCLSRMOVMOVWMULMULSMULSUNEGNOPORORIOUTPOPPUSHRCALLRETRETIRJMPRORSBCSBCISBISBICSBISSBIWSBRCSBRSSLEEPSPMSPM_2ST_XST_X_INCST_X_DECST_YST_Y_INCST_Y_DECSTD_YST_ZST_Z_INCST_Z_DECSTD_ZSTSSTS_SHORTSUBSUBISWAPWDRXCH"

var _Instruction_index = [...]uint16{0, 3, 6, 10, 13, 17, 20, 24, 27, 31, 35, 40, 44, 47, 51, 54, 57, 59, 62, 65, 69, 72, 75, 81, 86, 93, 97, 105, 108, 112, 117, 123, 128, 132, 134, 137, 140, 143, 146, 149, 153, 161, 169, 173, 181, 189, 194, 198, 206, 214, 219, 222, 225, 234, 240, 243, 250, 253, 256, 260, 263, 267, 272, 275, 278, 280, 283, 286, 289, 293, 298, 301, 305, 309, 312, 315, 319, 322, 326, 330, 334, 338, 342, 347, 350, 355, 359, 367, 375, 379, 387, 395, 400, 404, 412, 420, 425, 428, 437, 440, 444, 448, 451, 454}

func (i Instruction) String() string {
    if i < 0 || i+1 >= Instruction(len(_Instruction_index)) {
        return fmt.Sprintf("Instruction(%d)", i)
    }
    return _Instruction_name[_Instruction_index[i]:_Instruction_index[i+1]]
}
