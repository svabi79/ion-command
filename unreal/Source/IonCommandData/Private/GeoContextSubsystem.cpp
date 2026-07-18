#include "GeoContextSubsystem.h"

#include "GeoDataSubsystem.h"
#include "GeoMathLibrary.h"

TArray<FGeoMessageEnvelope> UGeoContextSubsystem::QueryNearby(const FGeoPosition& Center, double RadiusKm, const FString& SemanticTypePrefix) const
{
    TArray<FGeoMessageEnvelope> Result;
    const UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>();
    if (!Data || RadiusKm < 0.0) return Result;
    for (const FGeoMessageEnvelope& Message : Data->GetActiveMessages())
    {
        if (Message.Geometry.Positions.IsEmpty() || (!SemanticTypePrefix.IsEmpty() && !Message.SemanticType.StartsWith(SemanticTypePrefix))) continue;
        if (UGeoMathLibrary::GreatCircleDistanceKm(Center, Message.Geometry.Positions[0]) <= RadiusKm) Result.Add(Message);
    }
    return Result;
}

TArray<FGeoMessageEnvelope> UGeoContextSubsystem::QueryTimeRange(FDateTime FromUtc, FDateTime ToUtc, const FString& SemanticTypePrefix) const
{
    TArray<FGeoMessageEnvelope> Result;
    const UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>();
    if (!Data) return Result;
    for (const FGeoMessageEnvelope& Message : Data->GetActiveMessages())
    {
        if (Message.Time.ObservedUtc < FromUtc || Message.Time.ObservedUtc > ToUtc) continue;
        if (!SemanticTypePrefix.IsEmpty() && !Message.SemanticType.StartsWith(SemanticTypePrefix)) continue;
        Result.Add(Message);
    }
    return Result;
}

TArray<FGeoMessageEnvelope> UGeoContextSubsystem::QueryEntityRelationships(const FString& EntityId) const
{
    TArray<FGeoMessageEnvelope> Result;
    const UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>();
    if (!Data || EntityId.IsEmpty()) return Result;
    for (const FGeoMessageEnvelope& Message : Data->GetActiveMessages())
    {
        if (Message.MessageType == EGeoMessageType::Relationship && (Message.FromEntityId == EntityId || Message.ToEntityId == EntityId)) Result.Add(Message);
    }
    return Result;
}

