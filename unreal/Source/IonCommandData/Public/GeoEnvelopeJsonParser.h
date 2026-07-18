#pragma once

#include "CoreMinimal.h"
#include "GeoTypes.h"

class IONCOMMANDDATA_API FGeoEnvelopeJsonParser
{
public:
    static bool Parse(const FString& Json, FGeoMessageEnvelope& OutEnvelope, FString& OutError);
};

