-- Update existing traffic_metrics records with Valencia, Spain geographic data
-- This script updates records where country or other geographic fields are empty/null

-- Update records with empty or null geographic data to Valencia, Spain
UPDATE traffic_metrics 
SET 
    -- Geographic location information for Valencia, Spain
    geo_location = 'Valencia, Spain',
    latitude = 39.4699,
    longitude = -0.3763,
    city = 'Valencia',
    zip_code = '46001',
    country = 'Spain',
    country_code = 'ES',
    region = 'Valencia',
    continent = 'Europe',
    
    -- Set some default IP address if empty (Valencia-based IP range)
    ip_address = CASE 
        WHEN ip_address IS NULL OR ip_address = '' 
        THEN '185.174.' || (ABS(RANDOM()) % 256) || '.' || (ABS(RANDOM()) % 256)
        ELSE ip_address 
    END,
    
    -- Set default user agent if empty
    user_agent = CASE 
        WHEN user_agent IS NULL OR user_agent = '' 
        THEN 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
        ELSE user_agent 
    END,
    
    -- Set browser information if empty
    browser_family = CASE 
        WHEN browser_family IS NULL OR browser_family = '' 
        THEN 'Chrome'
        ELSE browser_family 
    END,
    
    browser_version = CASE 
        WHEN browser_version IS NULL OR browser_version = '' 
        THEN '120.0'
        ELSE browser_version 
    END,
    
    -- Set OS information if empty
    os_family = CASE 
        WHEN os_family IS NULL OR os_family = '' 
        THEN 'Windows'
        ELSE os_family 
    END,
    
    os_version = CASE 
        WHEN os_version IS NULL OR os_version = '' 
        THEN '10'
        ELSE os_version 
    END,
    
    -- Set device information if empty
    device_family = CASE 
        WHEN device_family IS NULL OR device_family = '' 
        THEN 'Desktop'
        ELSE device_family 
    END,
    
    device_brand = CASE 
        WHEN device_brand IS NULL OR device_brand = '' 
        THEN 'Generic'
        ELSE device_brand 
    END,
    
    device_model = CASE 
        WHEN device_model IS NULL OR device_model = '' 
        THEN 'PC'
        ELSE device_model 
    END,
    
    -- Set referrer if empty
    referrer = CASE 
        WHEN referrer IS NULL OR referrer = '' 
        THEN 'https://google.es'
        ELSE referrer 
    END,
    
    -- Update the updated_at timestamp
    updated_at = datetime('now')

WHERE 
    -- Only update records where geographic data is missing
    (country IS NULL OR country = '' OR 
     latitude = 0 OR latitude IS NULL OR 
     longitude = 0 OR longitude IS NULL OR
     city IS NULL OR city = '' OR
     country_code IS NULL OR country_code = '');

-- Verify the update by showing a count of updated records
SELECT 
    'Records updated with Valencia location data' as message,
    COUNT(*) as count
FROM traffic_metrics 
WHERE country = 'Spain' AND city = 'Valencia';

-- Show sample of updated records
SELECT 
    id,
    path,
    country,
    city,
    latitude,
    longitude,
    ip_address,
    browser_family,
    os_family,
    timestamp
FROM traffic_metrics 
WHERE country = 'Spain' AND city = 'Valencia'
LIMIT 5;
