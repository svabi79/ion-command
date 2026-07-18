#include "GeoEnvelopeJsonParser.h"

#include "Dom/JsonObject.h"
#include "Serialization/JsonReader.h"
#include "Serialization/JsonSerializer.h"

namespace
{
EGeoMessageType ParseMessageType(const FString& Value)
{
    if (Value == TEXT("entity")) return EGeoMessageType::Entity;
    if (Value == TEXT("observation")) return EGeoMessageType::Observation;
    if (Value == TEXT("relationship")) return EGeoMessageType::Relationship;
    if (Value == TEXT("track")) return EGeoMessageType::Track;
    if (Value == TEXT("area")) return EGeoMessageType::Area;
    if (Value == TEXT("field")) return EGeoMessageType::Field;
    if (Value == TEXT("volume")) return EGeoMessageType::Volume;
    if (Value == TEXT("annotation")) return EGeoMessageType::Annotation;
    return EGeoMessageType::Unknown;
}

EGeoGeometryType ParseGeometryType(const FString& Value)
{
    if (Value == TEXT("None")) return EGeoGeometryType::None;
    if (Value == TEXT("Point")) return EGeoGeometryType::Point;
    if (Value == TEXT("MultiPoint")) return EGeoGeometryType::MultiPoint;
    if (Value == TEXT("LineString")) return EGeoGeometryType::LineString;
    if (Value == TEXT("GreatCircle")) return EGeoGeometryType::GreatCircle;
    if (Value == TEXT("Arc")) return EGeoGeometryType::Arc;
    if (Value == TEXT("Polygon")) return EGeoGeometryType::Polygon;
    if (Value == TEXT("MultiPolygon")) return EGeoGeometryType::MultiPolygon;
    if (Value == TEXT("BoundingBox")) return EGeoGeometryType::BoundingBox;
    if (Value == TEXT("Circle")) return EGeoGeometryType::Circle;
    if (Value == TEXT("Track")) return EGeoGeometryType::Track;
    if (Value == TEXT("Raster")) return EGeoGeometryType::Raster;
    if (Value == TEXT("Grid")) return EGeoGeometryType::Grid;
    if (Value == TEXT("Volume")) return EGeoGeometryType::Volume;
    if (Value == TEXT("Shell")) return EGeoGeometryType::Shell;
    return EGeoGeometryType::Unknown;
}

bool ParseUtcField(const TSharedPtr<FJsonObject>& Object, const TCHAR* Name, FDateTime& OutValue, bool bRequired, FString& OutError)
{
    FString Text;
    if (!Object.IsValid() || !Object->TryGetStringField(Name, Text))
    {
        if (bRequired) OutError = FString::Printf(TEXT("missing time.%s"), Name);
        return !bRequired;
    }
    if (!FDateTime::ParseIso8601(*Text, OutValue))
    {
        OutError = FString::Printf(TEXT("invalid UTC value in time.%s"), Name);
        return false;
    }
    return true;
}

bool JsonValueToString(const TSharedPtr<FJsonValue>& Value, FString& Out)
{
    if (!Value.IsValid()) return false;
    switch (Value->Type)
    {
    case EJson::String: Out = Value->AsString(); return true;
    case EJson::Number: Out = FString::SanitizeFloat(Value->AsNumber()); return true;
    case EJson::Boolean: Out = Value->AsBool() ? TEXT("true") : TEXT("false"); return true;
    case EJson::Null: Out = TEXT("null"); return true;
    default:
        TSharedRef<TJsonWriter<>> Writer = TJsonWriterFactory<>::Create(&Out);
        return FJsonSerializer::Serialize(Value, TEXT(""), Writer);
    }
}

bool ParsePosition(const TArray<TSharedPtr<FJsonValue>>& Values, FGeoPosition& OutPosition)
{
    if (Values.Num() < 2 || Values[0]->Type != EJson::Number || Values[1]->Type != EJson::Number) return false;
    OutPosition.Longitude = Values[0]->AsNumber();
    OutPosition.Latitude = Values[1]->AsNumber();
    OutPosition.AltitudeMeters = Values.Num() > 2 && Values[2]->Type == EJson::Number ? Values[2]->AsNumber() : 0.0;
    return OutPosition.Longitude >= -180.0 && OutPosition.Longitude <= 180.0 && OutPosition.Latitude >= -90.0 && OutPosition.Latitude <= 90.0;
}
}

bool FGeoEnvelopeJsonParser::Parse(const FString& Json, FGeoMessageEnvelope& OutEnvelope, FString& OutError)
{
    OutEnvelope = FGeoMessageEnvelope();
    OutError.Reset();
    TSharedPtr<FJsonObject> Root;
    const TSharedRef<TJsonReader<>> Reader = TJsonReaderFactory<>::Create(Json);
    if (!FJsonSerializer::Deserialize(Reader, Root) || !Root.IsValid())
    {
        OutError = TEXT("invalid JSON object");
        return false;
    }
    double SchemaVersion = 0;
    FString MessageType;
    if (!Root->TryGetNumberField(TEXT("schemaVersion"), SchemaVersion) || !Root->TryGetStringField(TEXT("messageId"), OutEnvelope.MessageId) || !Root->TryGetStringField(TEXT("messageType"), MessageType) || !Root->TryGetStringField(TEXT("domain"), OutEnvelope.Domain) || !Root->TryGetStringField(TEXT("semanticType"), OutEnvelope.SemanticType))
    {
        OutError = TEXT("missing canonical envelope fields");
        return false;
    }
    OutEnvelope.SchemaVersion = static_cast<int32>(SchemaVersion);
    OutEnvelope.MessageType = ParseMessageType(MessageType);
    if (OutEnvelope.SchemaVersion != 1 || OutEnvelope.MessageType == EGeoMessageType::Unknown)
    {
        OutError = TEXT("unsupported schemaVersion or messageType");
        return false;
    }
    Root->TryGetStringField(TEXT("entityId"), OutEnvelope.EntityId);
    Root->TryGetStringField(TEXT("fromEntityId"), OutEnvelope.FromEntityId);
    Root->TryGetStringField(TEXT("toEntityId"), OutEnvelope.ToEntityId);
    Root->TryGetStringField(TEXT("targetId"), OutEnvelope.TargetId);

    const TSharedPtr<FJsonObject>* Source = nullptr;
    if (!Root->TryGetObjectField(TEXT("source"), Source) || !(*Source)->TryGetStringField(TEXT("pluginId"), OutEnvelope.Source.PluginId) || !(*Source)->TryGetStringField(TEXT("instanceId"), OutEnvelope.Source.InstanceId))
    {
        OutError = TEXT("missing source reference");
        return false;
    }
    (*Source)->TryGetStringField(TEXT("originalId"), OutEnvelope.Source.OriginalId);

    const TSharedPtr<FJsonObject>* Time = nullptr;
    if (!Root->TryGetObjectField(TEXT("time"), Time) || !ParseUtcField(*Time, TEXT("observedUtc"), OutEnvelope.Time.ObservedUtc, true, OutError) || !ParseUtcField(*Time, TEXT("receivedUtc"), OutEnvelope.Time.ReceivedUtc, true, OutError))
    {
        return false;
    }
    ParseUtcField(*Time, TEXT("validFromUtc"), OutEnvelope.Time.ValidFromUtc, false, OutError);
    ParseUtcField(*Time, TEXT("processingUtc"), OutEnvelope.Time.ProcessingUtc, false, OutError);
    FString ValidUntil;
    if ((*Time)->TryGetStringField(TEXT("validUntilUtc"), ValidUntil))
    {
        OutEnvelope.Time.bHasValidUntil = FDateTime::ParseIso8601(*ValidUntil, OutEnvelope.Time.ValidUntilUtc);
    }

    const TSharedPtr<FJsonObject>* Geometry = nullptr;
    if (Root->TryGetObjectField(TEXT("geometry"), Geometry))
    {
        (*Geometry)->TryGetStringField(TEXT("type"), OutEnvelope.Geometry.SourceType);
        (*Geometry)->TryGetStringField(TEXT("crs"), OutEnvelope.Geometry.CRS);
        OutEnvelope.Geometry.Type = ParseGeometryType(OutEnvelope.Geometry.SourceType);
        const TArray<TSharedPtr<FJsonValue>>* Coordinates = nullptr;
        if ((*Geometry)->TryGetArrayField(TEXT("coordinates"), Coordinates))
        {
            if (OutEnvelope.Geometry.Type == EGeoGeometryType::Point)
            {
                FGeoPosition Position;
                if (!ParsePosition(*Coordinates, Position)) { OutError = TEXT("invalid Point coordinates"); return false; }
                OutEnvelope.Geometry.Positions.Add(Position);
            }
            else if (OutEnvelope.Geometry.Type == EGeoGeometryType::GreatCircle || OutEnvelope.Geometry.Type == EGeoGeometryType::LineString)
            {
                for (const TSharedPtr<FJsonValue>& Coordinate : *Coordinates)
                {
                    const TArray<TSharedPtr<FJsonValue>>* PositionValues = nullptr;
                    if (!Coordinate->TryGetArray(PositionValues)) { OutError = TEXT("invalid line coordinates"); return false; }
                    FGeoPosition Position;
                    if (!ParsePosition(*PositionValues, Position)) { OutError = TEXT("invalid line position"); return false; }
                    OutEnvelope.Geometry.Positions.Add(Position);
                }
                if (OutEnvelope.Geometry.Positions.Num() < 2) { OutError = TEXT("line requires two or more positions"); return false; }
            }
        }
    }

    const TSharedPtr<FJsonObject>* Properties = nullptr;
    if (Root->TryGetObjectField(TEXT("properties"), Properties))
    {
        for (const TPair<FString, TSharedPtr<FJsonValue>>& Pair : (*Properties)->Values)
        {
            FString Value;
            if (JsonValueToString(Pair.Value, Value)) OutEnvelope.Properties.Add(Pair.Key, MoveTemp(Value));
        }
    }
    return true;
}

