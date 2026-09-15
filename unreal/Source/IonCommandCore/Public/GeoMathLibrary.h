#pragma once

#include "CoreMinimal.h"
#include "Kismet/BlueprintFunctionLibrary.h"
#include "GeoTypes.h"
#include "GeoMathLibrary.generated.h"

UCLASS()
class IONCOMMANDCORE_API UGeoMathLibrary final : public UBlueprintFunctionLibrary
{
    GENERATED_BODY()

public:
    static constexpr double EarthRadiusKm = 6371.0088;

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Geo")
    static FVector LatitudeLongitudeToUnitSphere(double Latitude, double Longitude);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Geo")
    static FGeoPosition UnitSphereToLatitudeLongitude(const FVector& UnitPosition);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Geo")
    static bool MaidenheadToLatLon(const FString& Locator, FGeoPosition& OutPosition);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Geo")
    static double GreatCircleDistanceKm(const FGeoPosition& From, const FGeoPosition& To);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Geo")
    static double InitialBearingDegrees(const FGeoPosition& From, const FGeoPosition& To);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Geo")
    static FGeoPosition GreatCircleInterpolation(const FGeoPosition& From, const FGeoPosition& To, double Alpha);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Solar")
    static FGeoPosition SolarSubpoint(const FDateTime& Utc);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Solar")
    static bool IsDaylight(const FGeoPosition& Position, const FDateTime& Utc);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Solar")
    static double GraylineDistanceKm(const FGeoPosition& Position, const FDateTime& Utc);

    // True when the globe sphere sits strictly between Eye and Point, so a
    // far-side marker/edge is occluded the same way the terrain hides it.
    // Not a UFUNCTION: pick helper only, no Blueprint need, no UHT churn.
    static bool IsOccludedByGlobe(const FVector& Eye, const FVector& Point, double GlobeRadius);
};

