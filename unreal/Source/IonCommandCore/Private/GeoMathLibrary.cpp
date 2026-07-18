#include "GeoMathLibrary.h"

namespace
{
double NormalizeDegrees(double Value)
{
    Value = FMath::Fmod(Value + 180.0, 360.0);
    if (Value < 0.0)
    {
        Value += 360.0;
    }
    return Value - 180.0;
}

double AngularDistanceRadians(const FGeoPosition& From, const FGeoPosition& To)
{
    const double Lat1 = FMath::DegreesToRadians(From.Latitude);
    const double Lat2 = FMath::DegreesToRadians(To.Latitude);
    const double DeltaLat = Lat2 - Lat1;
    const double DeltaLon = FMath::DegreesToRadians(To.Longitude - From.Longitude);
    const double A = FMath::Square(FMath::Sin(DeltaLat * 0.5)) + FMath::Cos(Lat1) * FMath::Cos(Lat2) * FMath::Square(FMath::Sin(DeltaLon * 0.5));
    return 2.0 * FMath::Atan2(FMath::Sqrt(A), FMath::Sqrt(FMath::Max(0.0, 1.0 - A)));
}
}

FVector UGeoMathLibrary::LatitudeLongitudeToUnitSphere(double Latitude, double Longitude)
{
    const double LatRad = FMath::DegreesToRadians(Latitude);
    const double LonRad = FMath::DegreesToRadians(Longitude);
    const double CosLat = FMath::Cos(LatRad);
    return FVector(CosLat * FMath::Cos(LonRad), CosLat * FMath::Sin(LonRad), FMath::Sin(LatRad));
}

FGeoPosition UGeoMathLibrary::UnitSphereToLatitudeLongitude(const FVector& UnitPosition)
{
    const FVector Normal = UnitPosition.GetSafeNormal();
    FGeoPosition Result;
    Result.Latitude = FMath::RadiansToDegrees(FMath::Asin(Normal.Z));
    Result.Longitude = FMath::RadiansToDegrees(FMath::Atan2(Normal.Y, Normal.X));
    return Result;
}

bool UGeoMathLibrary::MaidenheadToLatLon(const FString& Locator, FGeoPosition& OutPosition)
{
    const FString Value = Locator.TrimStartAndEnd().ToUpper();
    if (Value.Len() < 2 || Value.Len() % 2 != 0 || Value.Len() > 8)
    {
        return false;
    }
    auto LetterIndex = [](TCHAR Character, TCHAR Base, int32 Limit, int32& Out) -> bool
    {
        Out = static_cast<int32>(Character - Base);
        return Out >= 0 && Out < Limit;
    };
    int32 A = 0, B = 0;
    if (!LetterIndex(Value[0], TEXT('A'), 18, A) || !LetterIndex(Value[1], TEXT('A'), 18, B))
    {
        return false;
    }
    double Longitude = -180.0 + A * 20.0;
    double Latitude = -90.0 + B * 10.0;
    double CellLon = 20.0;
    double CellLat = 10.0;
    if (Value.Len() >= 4)
    {
        if (!FChar::IsDigit(Value[2]) || !FChar::IsDigit(Value[3]))
        {
            return false;
        }
        CellLon = 2.0;
        CellLat = 1.0;
        Longitude += (Value[2] - TEXT('0')) * CellLon;
        Latitude += (Value[3] - TEXT('0')) * CellLat;
    }
    if (Value.Len() >= 6)
    {
        if (!LetterIndex(Value[4], TEXT('A'), 24, A) || !LetterIndex(Value[5], TEXT('A'), 24, B))
        {
            return false;
        }
        CellLon = 2.0 / 24.0;
        CellLat = 1.0 / 24.0;
        Longitude += A * CellLon;
        Latitude += B * CellLat;
    }
    if (Value.Len() >= 8)
    {
        if (!FChar::IsDigit(Value[6]) || !FChar::IsDigit(Value[7]))
        {
            return false;
        }
        CellLon = (2.0 / 24.0) / 10.0;
        CellLat = (1.0 / 24.0) / 10.0;
        Longitude += (Value[6] - TEXT('0')) * CellLon;
        Latitude += (Value[7] - TEXT('0')) * CellLat;
    }
    OutPosition.Longitude = Longitude + CellLon * 0.5;
    OutPosition.Latitude = Latitude + CellLat * 0.5;
    OutPosition.AltitudeMeters = 0.0;
    return true;
}

double UGeoMathLibrary::GreatCircleDistanceKm(const FGeoPosition& From, const FGeoPosition& To)
{
    return EarthRadiusKm * AngularDistanceRadians(From, To);
}

double UGeoMathLibrary::InitialBearingDegrees(const FGeoPosition& From, const FGeoPosition& To)
{
    const double Lat1 = FMath::DegreesToRadians(From.Latitude);
    const double Lat2 = FMath::DegreesToRadians(To.Latitude);
    const double DeltaLon = FMath::DegreesToRadians(To.Longitude - From.Longitude);
    const double Y = FMath::Sin(DeltaLon) * FMath::Cos(Lat2);
    const double X = FMath::Cos(Lat1) * FMath::Sin(Lat2) - FMath::Sin(Lat1) * FMath::Cos(Lat2) * FMath::Cos(DeltaLon);
    return FMath::Fmod(FMath::RadiansToDegrees(FMath::Atan2(Y, X)) + 360.0, 360.0);
}

FGeoPosition UGeoMathLibrary::GreatCircleInterpolation(const FGeoPosition& From, const FGeoPosition& To, double Alpha)
{
    Alpha = FMath::Clamp(Alpha, 0.0, 1.0);
    const FVector A = LatitudeLongitudeToUnitSphere(From.Latitude, From.Longitude);
    const FVector B = LatitudeLongitudeToUnitSphere(To.Latitude, To.Longitude);
    const double Omega = FMath::Acos(FMath::Clamp(FVector::DotProduct(A, B), -1.0, 1.0));
    FVector Result;
    if (Omega < UE_DOUBLE_SMALL_NUMBER)
    {
        Result = A;
    }
    else
    {
        const double SinOmega = FMath::Sin(Omega);
        Result = (FMath::Sin((1.0 - Alpha) * Omega) / SinOmega) * A + (FMath::Sin(Alpha * Omega) / SinOmega) * B;
    }
    FGeoPosition Position = UnitSphereToLatitudeLongitude(Result);
    Position.AltitudeMeters = FMath::Lerp(From.AltitudeMeters, To.AltitudeMeters, Alpha);
    return Position;
}

FGeoPosition UGeoMathLibrary::SolarSubpoint(const FDateTime& Utc)
{
    const double JulianDate = static_cast<double>(Utc.ToUnixTimestamp()) / 86400.0 + 2440587.5 + static_cast<double>(Utc.GetMillisecond()) / 86400000.0;
    const double Days = JulianDate - 2451545.0;
    const double MeanAnomaly = FMath::DegreesToRadians(NormalizeDegrees(357.529 + 0.98560028 * Days));
    const double MeanLongitude = NormalizeDegrees(280.459 + 0.98564736 * Days);
    const double EclipticLongitude = FMath::DegreesToRadians(NormalizeDegrees(MeanLongitude + 1.915 * FMath::Sin(MeanAnomaly) + 0.020 * FMath::Sin(2.0 * MeanAnomaly)));
    const double Obliquity = FMath::DegreesToRadians(23.439 - 0.00000036 * Days);
    const double RightAscension = FMath::Atan2(FMath::Cos(Obliquity) * FMath::Sin(EclipticLongitude), FMath::Cos(EclipticLongitude));
    const double Declination = FMath::Asin(FMath::Sin(Obliquity) * FMath::Sin(EclipticLongitude));
    const double GreenwichSidereal = NormalizeDegrees(280.1600 + 360.9856235 * Days);
    FGeoPosition Result;
    Result.Latitude = FMath::RadiansToDegrees(Declination);
    Result.Longitude = NormalizeDegrees(FMath::RadiansToDegrees(RightAscension) - GreenwichSidereal);
    return Result;
}

bool UGeoMathLibrary::IsDaylight(const FGeoPosition& Position, const FDateTime& Utc)
{
    return AngularDistanceRadians(Position, SolarSubpoint(Utc)) < UE_HALF_PI;
}

double UGeoMathLibrary::GraylineDistanceKm(const FGeoPosition& Position, const FDateTime& Utc)
{
    const double AngularDistance = AngularDistanceRadians(Position, SolarSubpoint(Utc));
    return FMath::Abs(AngularDistance - UE_HALF_PI) * EarthRadiusKm;
}

