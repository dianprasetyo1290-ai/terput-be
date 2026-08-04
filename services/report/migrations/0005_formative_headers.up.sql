CREATE TABLE formative_headers (
    classroom_subject_id UUID NOT NULL,
    term_id UUID NOT NULL,
    order_index INT NOT NULL,
    header_name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (classroom_subject_id, term_id, order_index)
);
