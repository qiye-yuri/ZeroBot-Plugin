using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using Microsoft.Win32.SafeHandles;

namespace BatteryHub;

public record Reading(string Name, int? Percent, string Status, string Source);

internal static class Native
{
    [StructLayout(LayoutKind.Sequential)] struct Key { public Guid Fmt; public uint Id; public Key(string g,uint id) { Fmt=new Guid(g); Id=id; } }
    [DllImport("cfgmgr32.dll", CharSet=CharSet.Unicode)] static extern uint CM_Get_Device_ID_List_SizeW(out uint length, string? filter, uint flags);
    [DllImport("cfgmgr32.dll", CharSet=CharSet.Unicode)] static extern uint CM_Get_Device_ID_ListW(string? filter, [Out] char[] buffer, uint length, uint flags);
    [DllImport("cfgmgr32.dll", CharSet=CharSet.Unicode)] static extern uint CM_Locate_DevNodeW(out uint node, string id, uint flags);
    [DllImport("cfgmgr32.dll", CharSet=CharSet.Unicode)] static extern uint CM_Get_DevNode_PropertyW(uint node, ref Key key, out uint type, [Out] byte[] data, ref uint size, uint flags);
    static byte[]? Property(uint node, string guid, uint pid, out uint type) {
        var key=new Key(guid,pid); var b=new byte[4096]; uint n=(uint)b.Length;
        if(CM_Get_DevNode_PropertyW(node,ref key,out type,b,ref n,0)!=0) return null;
        Array.Resize(ref b,(int)n); return b;
    }
    const string DeviceKey="a45c254e-df1c-4efd-8020-67d146a850e0";
    static string TextProperty(uint node,uint pid) { var b=Property(node,DeviceKey,pid,out _); return b==null?"":Encoding.Unicode.GetString(b).TrimEnd('\0'); }
    public static Reading Headset() {
        bool? connected=ClassicConnected("EDIFIER Fit900NB");
        if(connected==false) return new("EDIFIER Fit900NB",null,"未连接 · 请开启耳机并连接蓝牙","蓝牙 / Windows");
        const uint present=0x100;
        if(CM_Get_Device_ID_List_SizeW(out uint length,null,present)!=0) throw new Exception("无法枚举蓝牙设备");
        char[] ids=new char[length];
        if(CM_Get_Device_ID_ListW(null,ids,length,present)!=0) throw new Exception("设备列表发生变化，请刷新");
        foreach(var id in new string(ids).Split('\0',StringSplitOptions.RemoveEmptyEntries)) {
            if(!id.StartsWith("BTH",StringComparison.OrdinalIgnoreCase)) continue;
            if(CM_Locate_DevNodeW(out uint node,id,0)!=0) continue;
            string name=TextProperty(node,14); if(name.Length==0) name=TextProperty(node,2);
            if(!name.Contains("Fit900NB",StringComparison.OrdinalIgnoreCase)) continue;
            var b=Property(node,"104ea319-6ee2-4701-bd47-8ddbf425bbe5",2,out uint type);
            if(b is {Length:1} && type==3 && b[0]<=100)
                return new("EDIFIER Fit900NB",b[0],connected==true?"已连接 · 系统最近上报电量":"系统电量 · 连接状态未知","蓝牙 / Windows");
        }
        return new("EDIFIER Fit900NB",null,"暂未获取电量 · 请连接耳机后刷新","蓝牙 / Windows");
    }
    [StructLayout(LayoutKind.Sequential)] struct Search { public uint Size; [MarshalAs(UnmanagedType.Bool)] public bool Auth,Remembered,Unknown,Connected,Inquiry; public byte Timeout; public IntPtr Radio; }
    [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] struct BtInfo {
        public uint Size; public ulong Address; public uint Class;
        [MarshalAs(UnmanagedType.Bool)] public bool Connected,Remembered,Authenticated;
        public SystemTime LastSeen,LastUsed;
        [MarshalAs(UnmanagedType.ByValTStr,SizeConst=248)] public string Name;
    }
    [StructLayout(LayoutKind.Sequential)] struct SystemTime { public ushort Year,Month,DayOfWeek,Day,Hour,Minute,Second,Milliseconds; }
    [DllImport("BluetoothApis.dll",SetLastError=true)] static extern IntPtr BluetoothFindFirstDevice(ref Search search,ref BtInfo info);
    [DllImport("BluetoothApis.dll",SetLastError=true)] [return:MarshalAs(UnmanagedType.Bool)] static extern bool BluetoothFindNextDevice(IntPtr handle,ref BtInfo info);
    [DllImport("BluetoothApis.dll")] static extern bool BluetoothFindDeviceClose(IntPtr handle);
    static bool? ClassicConnected(string name) {
        var s=new Search { Size=(uint)Marshal.SizeOf<Search>(),Auth=true,Remembered=true,Unknown=true,Connected=true };
        var d=new BtInfo { Size=(uint)Marshal.SizeOf<BtInfo>(),Name="" };
        var h=BluetoothFindFirstDevice(ref s,ref d); if(h==IntPtr.Zero) return Marshal.GetLastWin32Error()==259?false:null;
        try { do { if(d.Name.Equals(name,StringComparison.OrdinalIgnoreCase)) return d.Connected; } while(BluetoothFindNextDevice(h,ref d)); return false; }
        finally { BluetoothFindDeviceClose(h); }
    }
    [StructLayout(LayoutKind.Sequential)] struct InterfaceData { public uint Size; public Guid Guid; public uint Flags; public UIntPtr Reserved; }
    [DllImport("hid.dll")] static extern void HidD_GetHidGuid(out Guid guid);
    [DllImport("setupapi.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr SetupDiGetClassDevsW(ref Guid guid,string? enumerator,IntPtr parent,uint flags);
    [DllImport("setupapi.dll",SetLastError=true)] static extern bool SetupDiEnumDeviceInterfaces(IntPtr set,IntPtr device,ref Guid guid,uint index,ref InterfaceData data);
    [DllImport("setupapi.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool SetupDiGetDeviceInterfaceDetailW(IntPtr set,ref InterfaceData data,IntPtr detail,uint size,out uint required,IntPtr device);
    [DllImport("setupapi.dll")] static extern bool SetupDiDestroyDeviceInfoList(IntPtr set);
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern SafeFileHandle CreateFileW(string path,uint access,uint share,IntPtr security,uint creation,uint flags,IntPtr template);
    [DllImport("hid.dll",SetLastError=true)] static extern bool HidD_GetPreparsedData(SafeFileHandle h,out IntPtr data);
    [DllImport("hid.dll")] static extern bool HidD_FreePreparsedData(IntPtr data);
    [DllImport("hid.dll")] static extern int HidP_GetCaps(IntPtr data, [Out] byte[] caps);
    [DllImport("hid.dll",SetLastError=true)] static extern bool HidD_SetFeature(SafeFileHandle h,byte[] buffer,int length);
    [DllImport("hid.dll",SetLastError=true)] static extern bool HidD_GetFeature(SafeFileHandle h,[In,Out] byte[] buffer,int length);
    public static List<string> Diagnostics=new();
    static IEnumerable<string> HidPaths() {
        HidD_GetHidGuid(out var guid); var set=SetupDiGetClassDevsW(ref guid,null,IntPtr.Zero,0x12);
        if(set==new IntPtr(-1)) throw new Win32Exception();
        var paths=new List<string>();
        try { for(uint i=0;;i++) {
            var d=new InterfaceData { Size=(uint)Marshal.SizeOf<InterfaceData>() };
            if(!SetupDiEnumDeviceInterfaces(set,IntPtr.Zero,ref guid,i,ref d)) break;
            SetupDiGetDeviceInterfaceDetailW(set,ref d,IntPtr.Zero,0,out uint size,IntPtr.Zero);
            var p=Marshal.AllocHGlobal((int)size);
            try { Marshal.WriteInt32(p,IntPtr.Size==8?8:6); if(SetupDiGetDeviceInterfaceDetailW(set,ref d,p,size,out _,IntPtr.Zero)) paths.Add(Marshal.PtrToStringUni(p+4)!); }
            finally { Marshal.FreeHGlobal(p); }
        }} finally { SetupDiDestroyDeviceInfoList(set); }
        return paths;
    }
    public static byte[] BatteryRequest() {
        // Report ID prefix + 90-byte Razer query: class 0x07, command 0x80.
        var b=new byte[91]; b[2]=0x1f; b[6]=2; b[7]=7; b[8]=0x80;
        for(int i=3;i<=88;i++) b[89]^=b[i]; return b;
    }
    public static int? ParseBattery(byte[] b) {
        if(b.Length!=91 || b[1]!=2 || b[2]!=0x1f || b[6]!=2 || b[7]!=7 || b[8]!=0x80) return null;
        byte crc=0; for(int i=3;i<=88;i++) crc^=b[i];
        return crc==b[89]?(int)Math.Round(b[10]*100.0/255):null;
    }
    public static Reading Mouse() {
        Diagnostics.Clear(); bool found=false;
        foreach(var path in HidPaths()) {
            if(!path.Contains("vid_1532&pid_009c",StringComparison.OrdinalIgnoreCase)) continue;
            found=true;
            using var h=CreateFileW(path,0,3,IntPtr.Zero,3,0,IntPtr.Zero);
            if(h.IsInvalid) { Diagnostics.Add("打开失败: "+Marshal.GetLastWin32Error()); continue; }
            if(!HidD_GetPreparsedData(h,out var p)) continue;
            var caps=new byte[64]; int status; try { status=HidP_GetCaps(p,caps); } finally { HidD_FreePreparsedData(p); }
            int feature=BitConverter.ToUInt16(caps,8);
            Diagnostics.Add($"{path}: feature={feature}, caps={status:X}");
            if(status!=0x110000 || feature!=91) continue;
            for(int attempt=0;attempt<3;attempt++) {
                var request=BatteryRequest();
                if(!HidD_SetFeature(h,request,request.Length)) { Diagnostics.Add("查询失败: "+Marshal.GetLastWin32Error()); break; }
                Thread.Sleep(400);
                var reply=new byte[91];
                if(!HidD_GetFeature(h,reply,reply.Length)) { Diagnostics.Add("读取失败: "+Marshal.GetLastWin32Error()); break; }
                Diagnostics.Add("reply="+Convert.ToHexString(reply));
                int? percent=ParseBattery(reply);
                if(percent!=null) return new("DeathAdder V2 X HyperSpeed",percent,"已响应 · 接收器实时查询","雷蛇 / 2.4 GHz");
                if(reply[1]!=1 && reply[1]!=4) break;
            }
        }
        return new("DeathAdder V2 X HyperSpeed",null,found?"接收器已识别 · 鼠标休眠或暂未响应":"未发现接收器 · 请插入 USB 接收器","雷蛇 / 2.4 GHz");
    }
}
