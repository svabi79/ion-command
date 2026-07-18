#pragma once

#include "CoreMinimal.h"
#include "GeoTypes.generated.h"

UENUM(BlueprintType)
enum class EGeoMessageType : uint8
{
    Unknown,
    Entity,
    Observation,
    Relationship,
    Track,
    Area,
    Field,
    Volume,
    Annotation
};

UENUM(BlueprintType)
enum class EGeoGeometryType : uint8
{
    None,
    Point,
    MultiPoint,
    LineString,
    GreatCircle,
    Arc,
    Polygon,
    MultiPolygon,
    BoundingBox,
    Circle,
    Track,
    Raster,
    Grid,
    Volume,
    Shell,
    Unknown
};

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoPosition
{
    GENERATED_BODY()

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Geo")
    double Longitude = 0.0;

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Geo")
    double Latitude = 0.0;

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Geo")
    double AltitudeMeters = 0.0;
};

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoGeometry
{
    GENERATED_BODY()

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Geo")
    EGeoGeometryType Type = EGeoGeometryType::None;

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Geo")
    FString SourceType;

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Geo")
    FString CRS = TEXT("EPSG:4326");

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Geo")
    TArray<FGeoPosition> Positions;
};

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoSourceRef
{
    GENERATED_BODY()

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Source")
    FString PluginId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Source")
    FString InstanceId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Source")
    FString OriginalId;
};

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoTimeRange
{
    GENERATED_BODY()

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Time")
    FDateTime ObservedUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Time")
    FDateTime ReceivedUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Time")
    FDateTime ValidFromUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Time")
    FDateTime ValidUntilUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Time")
    FDateTime ProcessingUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Time")
    bool bHasValidUntil = false;
};

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoMessageEnvelope
{
    GENERATED_BODY()

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    int32 SchemaVersion = 0;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FString MessageId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    EGeoMessageType MessageType = EGeoMessageType::Unknown;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FString Domain;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FString SemanticType;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FString EntityId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FString FromEntityId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FString ToEntityId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FString TargetId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FGeoSourceRef Source;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FGeoTimeRange Time;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    FGeoGeometry Geometry;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Message")
    TMap<FString, FString> Properties;

    bool IsRenderable() const
    {
        return Geometry.Type != EGeoGeometryType::None && Geometry.Type != EGeoGeometryType::Unknown;
    }
};

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoRuntimeStatistics
{
    GENERATED_BODY()

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 AcceptedMessages = 0;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 InvalidMessages = 0;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 DroppedMessages = 0;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int32 ActiveMessages = 0;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int32 PendingMessages = 0;
};

