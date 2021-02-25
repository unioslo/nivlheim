package main

import (
	"database/sql"
	"os"
	"testing"
)

func TestParseFilesLinux(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	// Create a database connection
	db := getDBconnForTesting(t)
	defer db.Close()
	// Set up some test data
	expectedKernel := "4.15.13-300.fc27.x86_64"
	type file struct {
		filename, content string
	}
	testfiles := []file{
		{
			filename: "/bin/uname -r",
			content:  expectedKernel,
		},
		{
			filename: "/usr/sbin/dmidecode -t system",
			content:  dmiDecodeOutput,
		},
	}
	for _, f := range testfiles {
		_, err := db.Exec("INSERT INTO files(certfp,filename,content,received) "+
			"VALUES('1234',$1,$2,now())", f.filename, f.content)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := db.Exec("INSERT INTO customfields(name,filename,regexp) " +
		"VALUES('foo', '" + testfiles[0].filename + "', '(.*)')")
	if err != nil {
		t.Fatal(err)
	}

	// run the parseFiles Job
	job := parseFilesJob{}
	job.Run(db)

	// verify the results
	var kernel, manufacturer, product, serial sql.NullString
	err = db.QueryRow("SELECT kernel,manufacturer,product,serialno "+
		"FROM hostinfo WHERE certfp='1234'").
		Scan(&kernel, &manufacturer, &product, &serial)
	if err == sql.ErrNoRows {
		t.Fatal("No hostinfo row found")
	}
	if err != nil {
		t.Fatal(err)
	}

	if kernel.String != expectedKernel {
		t.Errorf("Kernel = %s, expected %s", kernel.String, expectedKernel)
	}

	expectedManufacturer := "Dell Inc."
	if manufacturer.String != expectedManufacturer {
		t.Errorf("Manufacturer = %s, expected %s", manufacturer.String, expectedManufacturer)
	}

	expectedProduct := "Latitude E7240"
	if product.String != expectedProduct {
		t.Errorf("Product = %s, expected %s", product.String, expectedProduct)
	}

	expectedSerial := "AFK5678"
	if serial.String != expectedSerial {
		t.Errorf("Serial no = %s, expected %s", serial.String, expectedSerial)
	}

	var v sql.NullString
	err = db.QueryRow("SELECT value FROM hostinfo_customfields " +
		"WHERE certfp='1234' AND fieldid=1").Scan(&v)
	switch {
	case err == sql.ErrNoRows:
		t.Errorf("The custom field wasn't parsed.")
	case err != nil:
		t.Fatal(err)
	}
}

func TestParseFilesWindows(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	// Create a database connection
	db := getDBconnForTesting(t)
	defer db.Close()
	// Set up some test data
	type file struct {
		filename, content string
	}
	testfiles := []file{
		{
			filename: "Get-WmiObject Win32_computersystemproduct|Select Name,Vendor|ConvertTo-Json",
			content: `{
				"Name": "HP ProDesk 600 G2 DM",
				"Vendor": "HP"
			}`,
		},
		{
			filename: "Get-WmiObject Win32_bios|Select smbiosbiosversion,manufacturer,name,serialnumber,version|ConvertTo-Json",
			content: `{
				"name": "Default System BIOS",
				"serialnumber": "CZC712345678",
				"version": "HPQOEM - 0"
			}`,
		},
		{
			filename: "[System.Environment]::OSVersion|ConvertTo-Json",
			content: `{
				"Platform": 2,
				"ServicePack": "",
				"Version": {
					"Major": 10,
					"Minor": 0,
					"Build": 17763,
					"Revision": 0,
					"MajorRevision": 0,
					"MinorRevision": 0
					},
				"VersionString": "Microsoft Windows NT 10.0.17763.0"
			}`,
		},
	}
	for _, f := range testfiles {
		_, err := db.Exec("INSERT INTO files(certfp,filename,content,received) "+
			"VALUES('1234',$1,$2,now())", f.filename, f.content)
		if err != nil {
			t.Fatal(err)
		}
	}

	// run the parseFiles Job
	job := parseFilesJob{}
	job.Run(db)

	// verify the results
	var kernel, manufacturer, product, serial sql.NullString
	err := db.QueryRow("SELECT kernel,manufacturer,product,serialno "+
		"FROM hostinfo WHERE certfp='1234'").
		Scan(&kernel, &manufacturer, &product, &serial)
	if err == sql.ErrNoRows {
		t.Fatal("No hostinfo row found")
	}
	if err != nil {
		t.Fatal(err)
	}

	expectedManufacturer := "HP"
	if manufacturer.String != expectedManufacturer {
		t.Errorf("Manufacturer = %s, expected %s", manufacturer.String, expectedManufacturer)
	}

	expectedProduct := "HP ProDesk 600 G2 DM"
	if product.String != expectedProduct {
		t.Errorf("Product = %s, expected %s", product.String, expectedProduct)
	}

	expectedSerial := "CZC712345678"
	if serial.String != expectedSerial {
		t.Errorf("Serial no = %s, expected %s", serial.String, expectedSerial)
	}

	expectedKernel := "10.0.17763.0"
	if kernel.String != expectedKernel {
		t.Errorf("Kernel = %s, expected %s", kernel.String, expectedKernel)
	}
}

func TestParseFilesMacOS(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	// Create a database connection
	db := getDBconnForTesting(t)
	defer db.Close()

	// Set up some test data
	type file struct {
		filename, content string
	}
	testfiles := []file{
		{
			filename: "/usr/sbin/system_profiler SPHardwareDataType",
			content: `Hardware:

		Hardware Overview:

		  Model Name: Mac mini
		  Model Identifier: Macmini8,1
		  Processor Name: Intel Core i3
		  Processor Speed: 3,6 GHz
		  Number of Processors: 1
		  Total Number of Cores: 4
		  L2 Cache (per Core): 256 KB
		  L3 Cache: 6 MB
		  Memory: 8 GB
		  Boot ROM Version: 220.270.99.0.0 (iBridge: 16.16.6571.0.0,0)
		  Serial Number (system): C07Z40T1JYVY
		  Hardware UUID: E0853D3F-0083-555C-9D46-0985E06238A1`,
		},
		{
			filename: "/usr/bin/uname -a",
			content:  `Darwin casmac05.uio.no 18.7.0 Darwin Kernel Version 18.7.0: Tue Aug 20 16:57:14 PDT 2019; root:xnu-4903.271.2~2/RELEASE_X86_64 x86_64`,
		},
	}
	for _, f := range testfiles {
		_, err := db.Exec("INSERT INTO files(certfp,filename,content,received) "+
			"VALUES('1234',$1,$2,now())", f.filename, f.content)
		if err != nil {
			t.Fatal(err)
		}
	}

	// run the parseFiles Job
	job := parseFilesJob{}
	job.Run(db)

	// verify the results
	var kernel, manufacturer, product, serial sql.NullString
	err := db.QueryRow("SELECT kernel,manufacturer,product,serialno "+
		"FROM hostinfo WHERE certfp='1234'").
		Scan(&kernel, &manufacturer, &product, &serial)
	if err == sql.ErrNoRows {
		t.Fatal("No hostinfo row found")
	}
	if err != nil {
		t.Fatal(err)
	}

	expectedManufacturer := "Apple"
	if manufacturer.String != expectedManufacturer {
		t.Errorf("Manufacturer = %s, expected %s", manufacturer.String, expectedManufacturer)
	}

	expectedProduct := "Mac mini"
	if product.String != expectedProduct {
		t.Errorf("Product = %s, expected %s", product.String, expectedProduct)
	}

	expectedSerial := "C07Z40T1JYVY"
	if serial.String != expectedSerial {
		t.Errorf("Serial no = %s, expected %s", serial.String, expectedSerial)
	}

	expectedKernel := "18.7.0"
	if kernel.String != expectedKernel {
		t.Errorf("Kernel = %s, expected %s", kernel.String, expectedKernel)
	}
}

func TestOSdetection(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	// Create a database connection
	db := getDBconnForTesting(t)
	defer db.Close()

	type os struct {
		osLabel, filename, content string
	}
	tests := []os{
		{
			osLabel:  "RHEL 7",
			filename: "/etc/redhat-release",
			content:  "Red Hat Enterprise Linux Workstation release 7.4 (Maipo)",
		},
		{
			osLabel:  "RHEL 8",
			filename: "/etc/redhat-release",
			content:  "Red Hat Enterprise Linux release 8.0 (Ootpa)",
		},
		{
			osLabel:  "Fedora 27",
			filename: "/etc/redhat-release",
			content:  "Fedora release 27 (Twenty Seven)",
		},
		{
			osLabel:  "Debian 9",
			filename: "/etc/debian_version",
			content:  "9.3",
		},
		{
			osLabel:  "Ubuntu 16.04",
			filename: "/etc/lsb-release",
			content: `DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=16.04
DISTRIB_CODENAME=xenial
DISTRIB_DESCRIPTION="Ubuntu 16.04.4 LTS"`,
		},
		{
			osLabel:  "macOS 10.13",
			filename: "/usr/bin/sw_vers",
			content: `ProductName:    Mac OS X
ProductVersion: 10.13.3
BuildVersion:   17D102`,
		},
		{
			osLabel: "macOS 11.2",
			filename: "/usr/bin/sw_vers",
			content: `ProductName:	macOS
ProductVersion:	11.2.1
BuildVersion:	20D74`,
		},
		{
			osLabel:  "FreeBSD 11",
			filename: "/bin/freebsd-version -ku",
			content:  "11.1-RELEASE-p6",
		},
		{
			osLabel:  "Windows 7",
			filename: "(Get-WmiObject Win32_OperatingSystem).Caption",
			content:  "Microsoft Windows 7 Enterprise",
		},
		{
			osLabel:  "Windows 2008",
			filename: "(Get-WmiObject Win32_OperatingSystem).Caption",
			content:  "Microsoft® Windows Server® 2008 Standard",
		},
		{
			osLabel:  "Windows 2008 R2",
			filename: "(Get-WmiObject Win32_OperatingSystem).Caption",
			content:  "Microsoft Windows Server 2008 R2 Standard",
		},
		{
			osLabel:  "Windows 2012",
			filename: "(Get-WmiObject Win32_OperatingSystem).Caption",
			content:  "Microsoft Windows Server 2012 Standard",
		},
		{
			osLabel:  "Windows 2012 R2",
			filename: "(Get-WmiObject Win32_OperatingSystem).Caption",
			content:  "Microsoft Windows Server 2012 R2 Standard",
		},
		{
			osLabel:  "Windows 2016",
			filename: "(Get-WmiObject Win32_OperatingSystem).Caption",
			content:  "Microsoft Windows Server 2016 Standard",
		},
	}
	const fileID = 10000
	const certfp = "AA11"
	for _, test := range tests {
		_, err := db.Exec("INSERT INTO files(fileid,certfp,filename,content,received) "+
			"VALUES($1,$2,$3,$4,now())", fileID, certfp, test.filename, test.content)
		if err != nil {
			t.Fatal(err)
		}
		db.Exec("UPDATE hostinfo SET os=null, os_edition=null WHERE certfp=$1",
			certfp)
		parseFile(db, fileID)
		db.Exec("DELETE FROM files WHERE fileid=$1", fileID)
		var os, osEdition sql.NullString
		err = db.QueryRow("SELECT os,os_edition FROM hostinfo WHERE certfp=$1",
			certfp).Scan(&os, &osEdition)
		if err != nil {
			t.Fatal(err)
		}
		if os.String != test.osLabel {
			t.Errorf("OS is %s, expected %s", os.String, test.osLabel)
		}
	}
}

const dmiDecodeOutput = `# dmidecode 3.1
Getting SMBIOS data from sysfs.
SMBIOS 2.7 present.

Handle 0x0001, DMI type 1, 27 bytes
System Information
	Manufacturer: DELL INC.
	Product Name: LATITUDE E7240
	Version: 01
	Serial Number: AFK5678
	UUID: 4C4C4544-0054-4B10-804E-CAC04F565931
	Wake-up Type: Power Switch
	SKU Number: Latitude E7240
	Family: Not Specified

Handle 0x0024, DMI type 12, 5 bytes
System Configuration Options
	Option 1: To Be Filled By O.E.M.

Handle 0x0025, DMI type 15, 35 bytes
System Event Log
	Area Length: 4 bytes
	Header Start Offset: 0x0000
	Header Length: 2 bytes
	Data Start Offset: 0x0002
	Access Method: Indexed I/O, one 16-bit index port, one 8-bit data port
	Access Address: Index 0x046A, Data 0x046C
	Status: Invalid, Not Full
	Change Token: 0x00000000
	Header Format: No Header
	Supported Log Type Descriptors: 6
	Descriptor 1: End of log
	Data Format 1: OEM-specific
	Descriptor 2: End of log
	Data Format 2: OEM-specific
	Descriptor 3: End of log
	Data Format 3: OEM-specific
	Descriptor 4: End of log
	Data Format 4: OEM-specific
	Descriptor 5: End of log
	Data Format 5: OEM-specific
	Descriptor 6: End of log
	Data Format 6: OEM-specific

Handle 0x002D, DMI type 32, 20 bytes
System Boot Information
	Status: No errors detected
`
