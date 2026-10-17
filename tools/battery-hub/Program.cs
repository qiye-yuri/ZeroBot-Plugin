using System;
using System.Text.Json;
namespace BatteryHub;
internal static class Program {
    static Reading Read(Func<Reading> read, string name) {
        try { return read(); }
        catch(Exception) { return new(name, null, "读取失败", ""); }
    }
    static void Main() => Console.WriteLine(JsonSerializer.Serialize(new[] {
        Read(Native.Headset, "EDIFIER Fit900NB"),
        Read(Native.Mouse, "DeathAdder V2 X HyperSpeed")
    }));
}
