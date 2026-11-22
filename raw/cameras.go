package raw

// camera is an entry of the table of camera matrices: the models it is for,
// as the cameras name themselves, and the matrix from XYZ to the camera's
// colours under D65, times 10000.
type camera struct {
	models []string
	matrix [9]int16
}

// cameras are the colour matrices of Sony's cameras as dcraw 9.28 by Dave
// Coffin tabulates them in adobe_coeff, Adobe's for D65; see NOTICE. dcraw
// matches by the beginning of the name, which would give a later camera
// whose name begins the same, an A7 IV for the A7, a wrong matrix; here the
// models are named, from dcraw's comments where an entry stands for
// several. An SLT camera may add a V to its name. Of the NEX-3 and NEX-5,
// Adobe's matrices are taken, not dcraw's own measurements.
var cameras = []camera{
	{[]string{"DSC-RX100M2", "DSC-RX100M3", "DSC-RX100M4", "DSC-RX100M5"}, [9]int16{6596, -2079, -562, -4782, 13016, 1933, -970, 1581, 5181}},
	{[]string{"DSC-RX100"}, [9]int16{8651, -2754, -1057, -3464, 12207, 1373, -568, 1398, 4434}},
	{[]string{"DSC-RX10M4"}, [9]int16{7699, -2566, -629, -2967, 11270, 1928, -378, 1286, 4807}},
	{[]string{"DSC-RX10", "DSC-RX10M2", "DSC-RX10M3"}, [9]int16{6679, -1825, -745, -5047, 13256, 1953, -1580, 2422, 5183}},
	{[]string{"DSC-RX1RM2"}, [9]int16{6629, -1900, -483, -4618, 12349, 2550, -622, 1381, 6514}},
	{[]string{"DSC-RX1", "DSC-RX1R"}, [9]int16{6344, -1612, -462, -4863, 12477, 2681, -865, 1786, 6899}},
	{[]string{"DSC-RX0"}, [9]int16{9396, -3507, -843, -2497, 11111, 1572, -343, 1355, 5089}},
	{[]string{"DSLR-A100"}, [9]int16{9437, -2811, -774, -8405, 16215, 2290, -710, 596, 7181}},
	{[]string{"DSLR-A290"}, [9]int16{6038, -1484, -579, -9145, 16746, 2512, -875, 746, 7218}},
	{[]string{"DSLR-A200", "DSLR-A230"}, [9]int16{9847, -3091, -928, -8485, 16345, 2225, -715, 595, 7103}},
	{[]string{"DSLR-A300"}, [9]int16{9847, -3091, -928, -8485, 16345, 2225, -715, 595, 7103}},
	{[]string{"DSLR-A330"}, [9]int16{9847, -3091, -929, -8485, 16346, 2225, -714, 595, 7103}},
	{[]string{"DSLR-A350"}, [9]int16{6038, -1484, -578, -9146, 16746, 2513, -875, 746, 7217}},
	{[]string{"DSLR-A380"}, [9]int16{6038, -1484, -579, -9145, 16746, 2512, -875, 746, 7218}},
	{[]string{"DSLR-A390"}, [9]int16{6038, -1484, -579, -9145, 16746, 2512, -875, 746, 7218}},
	{[]string{"DSLR-A450"}, [9]int16{4950, -580, -103, -5228, 12542, 3029, -709, 1435, 7371}},
	{[]string{"DSLR-A580"}, [9]int16{5932, -1492, -411, -4813, 12285, 2856, -741, 1524, 6739}},
	{[]string{"DSLR-A500"}, [9]int16{6046, -1127, -278, -5574, 13076, 2786, -691, 1419, 7625}},
	{[]string{"DSLR-A550", "DSLR-A560"}, [9]int16{4950, -580, -103, -5228, 12542, 3029, -709, 1435, 7371}},
	{[]string{"DSLR-A700"}, [9]int16{5775, -805, -359, -8574, 16295, 2391, -1943, 2341, 7249}},
	{[]string{"DSLR-A850"}, [9]int16{5413, -1162, -365, -5665, 13098, 2866, -608, 1179, 8440}},
	{[]string{"DSLR-A900"}, [9]int16{5209, -1072, -397, -8845, 16120, 2919, -1618, 1803, 8654}},
	{[]string{"ILCA-68"}, [9]int16{6435, -1903, -536, -4722, 12449, 2550, -663, 1363, 6517}},
	{[]string{"ILCA-77M2"}, [9]int16{5991, -1732, -443, -4100, 11989, 2381, -704, 1467, 5992}},
	{[]string{"ILCA-99M2"}, [9]int16{6660, -1918, -471, -4613, 12398, 2485, -649, 1433, 6447}},
	{[]string{"ILCE-6300", "ILCE-6500"}, [9]int16{5973, -1695, -419, -3826, 11797, 2293, -639, 1398, 5789}},
	{[]string{"ILCE-7M2"}, [9]int16{5271, -712, -347, -6153, 13653, 2763, -1601, 2366, 7242}},
	{[]string{"ILCE-7M3"}, [9]int16{7374, -2389, -551, -5435, 13162, 2519, -1006, 1795, 6552}},
	{[]string{"ILCE-7S", "ILCE-7SM2"}, [9]int16{5838, -1430, -246, -3497, 11477, 2297, -748, 1885, 5778}},
	{[]string{"ILCE-7RM3"}, [9]int16{6640, -1847, -503, -5238, 13010, 2474, -993, 1673, 6527}},
	{[]string{"ILCE-7RM2"}, [9]int16{6629, -1900, -483, -4618, 12349, 2550, -622, 1381, 6514}},
	{[]string{"ILCE-7R"}, [9]int16{4913, -541, -202, -6130, 13513, 2906, -1564, 2151, 7183}},
	{[]string{"ILCE-7"}, [9]int16{5271, -712, -347, -6153, 13653, 2763, -1601, 2366, 7242}},
	{[]string{"ILCE-9"}, [9]int16{6389, -1703, -378, -4562, 12265, 2587, -670, 1489, 6550}},
	{[]string{"ILCE-3000", "ILCE-5000", "ILCE-5100", "ILCE-6000", "ILCE-QX1"}, [9]int16{5991, -1456, -455, -4764, 12135, 2980, -707, 1425, 6701}},
	{[]string{"NEX-5N"}, [9]int16{5991, -1456, -455, -4764, 12135, 2980, -707, 1425, 6701}},
	{[]string{"NEX-5R", "NEX-5T", "NEX-3N", "NEX-6"}, [9]int16{6129, -1545, -418, -4930, 12490, 2743, -977, 1693, 6615}},
	{[]string{"NEX-3", "NEX-5"}, [9]int16{6549, -1550, -436, -4880, 12435, 2753, -854, 1868, 6976}},
	{[]string{"NEX-7"}, [9]int16{5491, -1192, -363, -4951, 12342, 2948, -911, 1722, 7192}},
	{[]string{"NEX-C3", "NEX-F3"}, [9]int16{5991, -1456, -455, -4764, 12135, 2980, -707, 1425, 6701}},
	{[]string{"SLT-A33"}, [9]int16{6069, -1221, -366, -5221, 12779, 2734, -1024, 2066, 6834}},
	{[]string{"SLT-A35"}, [9]int16{5986, -1618, -415, -4557, 11820, 3120, -681, 1404, 6971}},
	{[]string{"SLT-A37", "SLT-A57", "SLT-A58"}, [9]int16{5991, -1456, -455, -4764, 12135, 2980, -707, 1425, 6701}},
	{[]string{"SLT-A55"}, [9]int16{5932, -1492, -411, -4813, 12285, 2856, -741, 1524, 6739}},
	{[]string{"SLT-A65", "SLT-A77"}, [9]int16{5491, -1192, -363, -4951, 12342, 2948, -911, 1722, 7192}},
	{[]string{"SLT-A99"}, [9]int16{6344, -1612, -462, -4863, 12477, 2681, -865, 1786, 6899}},
}

// cameraMatrix is the entry of the table for a Sony model, as the camera
// writes it into the file.
func cameraMatrix(model string) (*camera, bool) {
	for i := range cameras {
		for _, m := range cameras[i].models {
			if model == m || model == m+"V" {
				return &cameras[i], true
			}
		}
	}
	return nil, false
}
