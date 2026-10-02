-- v4 -> v5: Separate expiry worker leases from applicant answer deadlines [incompatible: earlier binaries reopen expired web tokens while leasing challenges]
ALTER TABLE challenge ADD COLUMN expiry_claim_until BIGINT;
